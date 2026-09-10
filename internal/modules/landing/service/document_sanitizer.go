package service

import (
	"regexp"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"

	"zyad.cloud/internal/modules/landing/domain"
)

// grapesHTMLPolicy is the allowlist for GrapesJS-authored page HTML. It is far
// broader than the section content policy (structural + layout + media tags are
// allowed) but still refuses anything executable: no <script>/<style>/<iframe>/
// <object>/<embed>/<link>/<meta>, no on* handlers, no javascript: URLs. Inline
// `style` attributes are permitted but their value is re-sanitized as CSS by
// sanitizeInlineStyles (bluemonday does not sanitize CSS declarations itself).
var grapesHTMLPolicy = buildGrapesHTMLPolicy()

func buildGrapesHTMLPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	p.AllowElements(
		"div", "section", "article", "aside", "header", "footer", "nav", "main",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"p", "span", "small", "strong", "em", "b", "i", "u", "s", "sub", "sup",
		"br", "hr", "blockquote", "pre", "code", "mark", "abbr", "time",
		"ul", "ol", "li", "dl", "dt", "dd",
		"a", "img", "figure", "figcaption", "picture", "source",
		"table", "thead", "tbody", "tfoot", "tr", "td", "th", "caption", "colgroup", "col",
		"button", "label",
		"details", "summary",
	)

	// Global structural / styling attributes.
	p.AllowAttrs("class", "id", "title", "role", "style", "dir", "lang").Globally()
	// Tenant-chrome sentinel (Phase 5b live nav/footer substitution) + the
	// per-page header presentation blob (Phase 8: sticky/variant/align/action).
	p.AllowAttrs("data-zyad-slot", "data-zyad-header").Globally()
	p.AllowAttrs(
		"aria-label", "aria-hidden", "aria-expanded", "aria-controls",
		"aria-describedby", "aria-labelledby", "aria-current",
	).Globally()

	p.AllowAttrs("href", "target", "rel").OnElements("a")
	p.AllowAttrs("src", "srcset", "alt", "width", "height", "loading", "decoding").OnElements("img")
	p.AllowAttrs("srcset", "src", "type", "media", "sizes").OnElements("source")
	p.AllowAttrs("colspan", "rowspan", "scope").OnElements("td", "th")
	p.AllowAttrs("span").OnElements("col", "colgroup")
	p.AllowAttrs("open").OnElements("details")

	p.AllowStandardURLs()
	p.AllowURLSchemes("http", "https", "mailto", "tel")
	p.AllowDataURIImages()
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.RequireNoReferrerOnFullyQualifiedLinks(true)

	return p
}

var (
	// Standalone executable / exfiltration keywords (url() targets are handled
	// separately by isSafeCSSURL).
	cssDangerRe = regexp.MustCompile(`(?i)(expression\s*\(|-moz-binding|behaviou?r\s*:|@import|@charset)`)
	cssURLRe    = regexp.MustCompile(`(?i)url\(\s*['"]?([^'")]*)['"]?\s*\)`)
	// Matches a style attribute and captures its (double- or single-quoted) value.
	inlineStyleRe = regexp.MustCompile(`(?i)\sstyle\s*=\s*"([^"]*)"|\sstyle\s*=\s*'([^']*)'`)
	safeRelPathRe = regexp.MustCompile(`^/[A-Za-z0-9._~!$&'()*+,;=:@%/-]*$`)
)

func isSafeCSSURL(raw string) bool {
	u := strings.TrimSpace(raw)
	if u == "" || strings.ContainsAny(u, "\\<>\"' \t\r\n") {
		return false
	}
	lu := strings.ToLower(u)
	if strings.Contains(lu, "javascript:") || strings.Contains(lu, "vbscript:") {
		return false
	}
	switch {
	case strings.HasPrefix(lu, "http://"), strings.HasPrefix(lu, "https://"):
		return true
	case strings.HasPrefix(lu, "data:image/"):
		return true
	case strings.HasPrefix(u, "#"):
		return true
	case strings.HasPrefix(u, "//"):
		return false
	default:
		return safeRelPathRe.MatchString(u)
	}
}

// sanitizeCSS strips executable / exfiltration constructs from a CSS string —
// used for both the exported stylesheet and inline `style` values. It is a
// conservative token scan, not a full parser; the public renderer's iframe runs
// without `allow-scripts` as the backstop.
func sanitizeCSS(css string) string {
	if strings.TrimSpace(css) == "" {
		return ""
	}
	// 1. Neutralise every url() whose target isn't demonstrably safe.
	css = cssURLRe.ReplaceAllStringFunc(css, func(match string) string {
		sub := cssURLRe.FindStringSubmatch(match)
		if len(sub) < 2 || !isSafeCSSURL(sub[1]) {
			return "url(about:blank)"
		}
		return match
	})
	// 2. Strip standalone dangerous keywords.
	css = cssDangerRe.ReplaceAllString(css, "")
	return css
}

func sanitizeInlineStyles(html string) string {
	return inlineStyleRe.ReplaceAllStringFunc(html, func(match string) string {
		sub := inlineStyleRe.FindStringSubmatch(match)
		value := sub[1]
		if value == "" {
			value = sub[2]
		}
		return ` style="` + sanitizeCSS(value) + `"`
	})
}

// SanitizeGrapesHTML returns page-builder HTML with every executable construct
// removed and inline styles re-sanitized as CSS.
func SanitizeGrapesHTML(html string) string {
	return sanitizeInlineStyles(grapesHTMLPolicy.Sanitize(html))
}

// SanitizeGrapesCSS returns the exported stylesheet with unsafe constructs removed.
func SanitizeGrapesCSS(css string) string {
	return sanitizeCSS(css)
}

// buildGrapesJSSnapshot is the landing_page_versions.snapshot payload for a
// published GrapesJS page. HTML/CSS are sanitized here so the public resolver
// can serve the snapshot verbatim.
func buildGrapesJSSnapshot(page domain.LandingPage, doc domain.LandingPageDocument, at time.Time) map[string]any {
	project := doc.Project
	if project == nil {
		project = map[string]any{}
	}
	return map[string]any{
		"builder":       string(domain.PageBuilderGrapesJS),
		"page":          page,
		"seo":           page.SEO,
		"html":          SanitizeGrapesHTML(doc.HTML),
		"css":           SanitizeGrapesCSS(doc.CSS),
		"project":       project,
		"snapshot_time": at,
	}
}

func grapesSnapshotMarkup(snapshot map[string]any) (html string, css string) {
	if snapshot == nil {
		return "", ""
	}
	if v, ok := snapshot["html"].(string); ok {
		html = v
	}
	if v, ok := snapshot["css"].(string); ok {
		css = v
	}
	return html, css
}
