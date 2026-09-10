package service

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"

	"zyad.cloud/internal/modules/landing/domain"
)

// document_ssr.go renders a published GrapesJS page as a full, script-free HTML
// document for crawlers / no-JS clients (GET /public/landing/render). The
// HTML/CSS in ResolvedPage is already sanitised by the resolver; here we only
// wrap it in <head> metadata and substitute the tenant-chrome sentinels
// (data-zyad-slot) server-side — the same job GrapesPageFrame.vue does client
// side, so both render identically.

const ssrBaseCSS = `*,*::before,*::after{box-sizing:border-box}
html,body{margin:0;padding:0}
img,video,iframe{max-width:100%}`

const ssrChromeCSS = `.zyad-tenant-header{display:flex;align-items:center;gap:24px;padding:14px 24px;font-family:'Inter','Segoe UI',system-ui,sans-serif}
.zyad-tenant-header--solid{background:#fff;border-bottom:1px solid #e5e7eb}
.zyad-tenant-header--glass{background:rgba(255,255,255,.72);backdrop-filter:blur(8px);border-bottom:1px solid #e5e7eb}
.zyad-tenant-header--transparent{background:transparent}
.zyad-tenant-header--sticky{position:sticky;top:0;z-index:50}
.zyad-tenant-header--left{justify-content:space-between}
.zyad-tenant-header--center{justify-content:center}
.zyad-tenant-header--right{justify-content:flex-end}
.zyad-tenant-header__brand{display:flex;align-items:center;gap:8px;font-weight:700;color:#0f172a;text-decoration:none;font-size:16px}
.zyad-tenant-header__brand img{height:28px;width:auto;display:block}
.zyad-tenant-header__nav{display:flex;align-items:center;gap:22px;flex-wrap:wrap}
.zyad-tenant-header__nav a{color:#475569;text-decoration:none;font-size:14px;font-weight:500}
.zyad-tenant-header__action{display:inline-block;padding:9px 18px;border-radius:8px;background:#2563eb;color:#fff;font-weight:600;font-size:14px;text-decoration:none;white-space:nowrap}
.zyad-tenant-footer{padding:48px 24px;background:#0f172a;color:#cbd5e1;font-size:14px}
.zyad-tenant-footer__top{max-width:1120px;margin:0 auto;display:flex;flex-wrap:wrap;gap:32px;justify-content:space-between}
.zyad-tenant-footer__brand{display:flex;align-items:center;gap:10px;color:#fff;font-weight:800;font-size:16px}
.zyad-tenant-footer__brand img{height:28px;width:auto}
.zyad-tenant-footer__title{color:#fff;font-weight:600;margin:0 0 10px}
.zyad-tenant-footer__col nav{display:flex;flex-direction:column;gap:6px}
.zyad-tenant-footer__col a{color:#cbd5e1;text-decoration:none}
.zyad-tenant-footer__copyright{max-width:1120px;margin:28px auto 0;border-top:1px solid #1e293b;padding-top:16px;font-size:13px}`

var (
	navSentinelRe    = regexp.MustCompile(`(?is)<div\b[^>]*\bdata-zyad-slot="tenant-nav"[^>]*>.*?</div>`)
	footerSentinelRe = regexp.MustCompile(`(?is)<div\b[^>]*\bdata-zyad-slot="tenant-footer"[^>]*>.*?</div>`)
	safeSSRHrefRe    = regexp.MustCompile(`^(#|/(?:[^/]|$)|https?://|mailto:|tel:)`)
	dataZyadHeaderRe = regexp.MustCompile(`(?is)\bdata-zyad-header\s*=\s*("([^"]*)"|'([^']*)')`)
)

// ssrHeaderPresentation mirrors the FE TenantHeaderPresentation (per-page).
type ssrHeaderPresentation struct {
	Sticky      bool   `json:"sticky"`
	Variant     string `json:"variant"`
	Align       string `json:"align"`
	ShowAction  bool   `json:"showAction"`
	ActionLabel string `json:"actionLabel"`
	ActionURL   string `json:"actionUrl"`
}

func defaultSSRHeaderPresentation() ssrHeaderPresentation {
	return ssrHeaderPresentation{
		Sticky: true, Variant: "solid", Align: "left",
		ShowAction: true, ActionLabel: "Masuk", ActionURL: "/login",
	}
}

// parseSSRHeaderPresentation pulls the data-zyad-header JSON out of a matched
// sentinel opening tag (bluemonday entity-encodes the value).
func parseSSRHeaderPresentation(sentinel string) ssrHeaderPresentation {
	p := defaultSSRHeaderPresentation()
	m := dataZyadHeaderRe.FindStringSubmatch(sentinel)
	if m == nil {
		return p
	}
	raw := m[2]
	if raw == "" {
		raw = m[3]
	}
	raw = html.UnescapeString(raw)
	if raw == "" {
		return p
	}
	// Unmarshal onto the defaults so keys absent from the JSON keep their default.
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return defaultSSRHeaderPresentation()
	}
	if p.Variant != "solid" && p.Variant != "transparent" && p.Variant != "glass" {
		p.Variant = "solid"
	}
	if p.Align != "left" && p.Align != "center" && p.Align != "right" {
		p.Align = "left"
	}
	return p
}

type ssrChromeLink struct {
	Label  string
	Href   string
	Target string
}

type ssrChromeColumn struct {
	Title string
	Links []ssrChromeLink
}

// RenderGrapesDocument builds the full HTML document for a resolved GrapesJS
// page. Returns "" when the page is not a GrapesJS page (no HTML to render).
func RenderGrapesDocument(resolved ResolvedPage) string {
	if strings.TrimSpace(resolved.HTML) == "" {
		return ""
	}

	page := resolved.Page
	title := seoString(page.SEO, "meta_title")
	if title == "" {
		title = page.Title
	}
	description := seoString(page.SEO, "meta_description")
	ogImage := ""
	if og, ok := page.SEO["open_graph"].(map[string]any); ok {
		if v, ok := og["image_url"].(string); ok {
			ogImage = strings.TrimSpace(v)
		}
	}

	robots := "index, follow"
	if page.Visibility != domain.PageVisibilityPublic {
		robots = "noindex, nofollow"
	}

	body := fillGrapesSentinels(resolved.HTML, resolved.Menus, resolved.Branding, page)
	css := strings.ReplaceAll(resolved.CSS, "</style", `<\/style`)

	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"id\">\n<head>\n")
	b.WriteString(`<meta charset="utf-8">` + "\n")
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">` + "\n")
	b.WriteString("<title>" + html.EscapeString(title) + "</title>\n")
	if description != "" {
		b.WriteString(`<meta name="description" content="` + html.EscapeString(description) + `">` + "\n")
	}
	b.WriteString(`<meta name="robots" content="` + robots + `">` + "\n")
	b.WriteString(`<meta property="og:type" content="website">` + "\n")
	if title != "" {
		b.WriteString(`<meta property="og:title" content="` + html.EscapeString(title) + `">` + "\n")
	}
	if description != "" {
		b.WriteString(`<meta property="og:description" content="` + html.EscapeString(description) + `">` + "\n")
	}
	if ogImage != "" && strings.HasPrefix(ogImage, "http") {
		b.WriteString(`<meta property="og:image" content="` + html.EscapeString(ogImage) + `">` + "\n")
	}
	b.WriteString("<style>\n" + ssrBaseCSS + "\n" + ssrChromeCSS + "\n" + css + "\n</style>\n")
	b.WriteString("</head>\n<body>\n")
	b.WriteString(body)
	b.WriteString("\n</body>\n</html>\n")
	return b.String()
}

func seoString(seo map[string]any, key string) string {
	if seo == nil {
		return ""
	}
	if v, ok := seo[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func fillGrapesSentinels(
	markup string,
	menus []ResolvedMenu,
	branding domain.LandingBranding,
	page domain.LandingPage,
) string {
	nav := headerChromeLinks(menus)
	markup = navSentinelRe.ReplaceAllStringFunc(markup, func(sentinel string) string {
		pres := parseSSRHeaderPresentation(sentinel)
		return `<div data-zyad-slot="tenant-nav" class="zyad-slot">` +
			buildHeaderMarkup(nav, branding, pres) + `</div>`
	})

	columns := footerChromeColumns(menus)
	replacement := `<div data-zyad-slot="tenant-footer" class="zyad-slot">` +
		buildFooterMarkup(branding, columns, footerCopyright(branding, page)) + `</div>`
	markup = footerSentinelRe.ReplaceAllStringFunc(markup, func(string) string { return replacement })

	return markup
}

func buildHeaderMarkup(links []ssrChromeLink, branding domain.LandingBranding, p ssrHeaderPresentation) string {
	var b strings.Builder
	b.WriteString(`<div class="zyad-tenant-header zyad-tenant-header--` + p.Variant +
		` zyad-tenant-header--` + p.Align)
	if p.Sticky {
		b.WriteString(` zyad-tenant-header--sticky`)
	}
	b.WriteString(`">`)

	b.WriteString(`<a class="zyad-tenant-header__brand" href="/">`)
	if logo := strings.TrimSpace(branding.LogoLightURL); (strings.HasPrefix(logo, "https://") ||
		strings.HasPrefix(logo, "http://")) && !strings.ContainsAny(logo, "\"'<> \t\r\n") {
		b.WriteString(`<img src="` + html.EscapeString(logo) + `" alt="` +
			html.EscapeString(branding.CompanyName) + `">`)
	}
	if branding.CompanyName != "" {
		b.WriteString(`<span>` + html.EscapeString(branding.CompanyName) + `</span>`)
	}
	b.WriteString(`</a>`)

	b.WriteString(`<nav class="zyad-tenant-header__nav">`)
	for _, link := range links {
		writeAnchor(&b, link)
	}
	b.WriteString(`</nav>`)

	if p.ShowAction && (p.ActionLabel != "" || p.ActionURL != "") {
		label := p.ActionLabel
		if label == "" {
			label = "Masuk"
		}
		b.WriteString(`<a class="zyad-tenant-header__action" href="` +
			html.EscapeString(safeSSRHref(p.ActionURL)) + `">` + html.EscapeString(label) + `</a>`)
	}

	b.WriteString(`</div>`)
	return b.String()
}

func buildFooterMarkup(branding domain.LandingBranding, columns []ssrChromeColumn, copyright string) string {
	var b strings.Builder
	b.WriteString(`<div class="zyad-tenant-footer"><div class="zyad-tenant-footer__top">`)

	b.WriteString(`<div class="zyad-tenant-footer__brand">`)
	if logo := strings.TrimSpace(branding.LogoLightURL); strings.HasPrefix(logo, "https://") ||
		strings.HasPrefix(logo, "http://") {
		if !strings.ContainsAny(logo, "\"'<> \t\r\n") {
			b.WriteString(`<img src="` + html.EscapeString(logo) + `" alt="` +
				html.EscapeString(branding.CompanyName) + `">`)
		}
	}
	if branding.CompanyName != "" {
		b.WriteString(`<span>` + html.EscapeString(branding.CompanyName) + `</span>`)
	}
	b.WriteString(`</div>`)

	for _, col := range columns {
		b.WriteString(`<div class="zyad-tenant-footer__col"><p class="zyad-tenant-footer__title">` +
			html.EscapeString(col.Title) + `</p><nav>`)
		for _, link := range col.Links {
			writeAnchor(&b, link)
		}
		b.WriteString(`</nav></div>`)
	}
	b.WriteString(`</div>`)

	if copyright != "" {
		b.WriteString(`<p class="zyad-tenant-footer__copyright">` + html.EscapeString(copyright) + `</p>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func writeAnchor(b *strings.Builder, link ssrChromeLink) {
	b.WriteString(`<a href="` + html.EscapeString(safeSSRHref(link.Href)) + `"`)
	if link.Target == "new_tab" || link.Target == "_blank" {
		b.WriteString(` target="_blank" rel="noopener noreferrer"`)
	}
	b.WriteString(`>` + html.EscapeString(link.Label) + `</a>`)
}

func headerChromeLinks(menus []ResolvedMenu) []ssrChromeLink {
	for _, menu := range menus {
		if menu.Location != "header" || !menu.IsActive {
			continue
		}
		items := append([]ResolvedMenuItem(nil), menu.Items...)
		sort.SliceStable(items, func(i, j int) bool { return items[i].SortOrder < items[j].SortOrder })

		links := make([]ssrChromeLink, 0, len(items))
		for _, item := range items {
			if !item.IsEnabled || item.Label == "" {
				continue
			}
			links = append(links, ssrChromeLink{
				Label:  item.Label,
				Href:   hrefForMenuItem(item.LinkType, item.Destination),
				Target: item.Target,
			})
		}
		return links
	}
	return nil
}

func footerChromeColumns(menus []ResolvedMenu) []ssrChromeColumn {
	var columns []ssrChromeColumn
	for _, menu := range menus {
		if menu.Location != "footer" || !menu.IsActive || len(menu.Items) == 0 {
			continue
		}
		items := append([]ResolvedMenuItem(nil), menu.Items...)
		sort.SliceStable(items, func(i, j int) bool { return items[i].SortOrder < items[j].SortOrder })

		links := make([]ssrChromeLink, 0, len(items))
		for _, item := range items {
			if !item.IsEnabled || item.Label == "" {
				continue
			}
			links = append(links, ssrChromeLink{
				Label: item.Label,
				Href:  hrefForMenuItem(item.LinkType, item.Destination),
			})
		}
		if len(links) > 0 {
			columns = append(columns, ssrChromeColumn{Title: menu.Name, Links: links})
		}
	}
	return columns
}

func footerCopyright(branding domain.LandingBranding, page domain.LandingPage) string {
	if page.Settings.FooterCopyrightText != "" {
		return page.Settings.FooterCopyrightText
	}
	name := branding.CompanyName
	if name == "" {
		name = page.Title
	}
	return fmt.Sprintf("© %d %s", time.Now().Year(), name)
}

func hrefForMenuItem(linkType, destination string) string {
	dest := strings.TrimSpace(destination)
	switch linkType {
	case "anchor":
		if strings.HasPrefix(dest, "#") {
			return dest
		}
		return "#" + dest
	case "internal_page", "button":
		if dest == "" || dest == "public-marketing" {
			return "/"
		}
		if strings.HasPrefix(dest, "/") {
			return dest
		}
		return "/" + dest
	default:
		if dest == "" {
			return "#"
		}
		return dest
	}
}

func safeSSRHref(raw string) string {
	href := strings.TrimSpace(raw)
	if href == "" || strings.HasPrefix(href, "//") {
		return "#"
	}
	if strings.ContainsAny(href, "\"'<>` \t\r\n") {
		return "#"
	}
	if safeSSRHrefRe.MatchString(href) {
		return href
	}
	return "#"
}
