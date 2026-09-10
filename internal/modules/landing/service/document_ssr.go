package service

import (
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

const ssrChromeCSS = `.zyad-tenant-nav{display:flex;flex-wrap:wrap;gap:8px 24px;align-items:center;padding:16px 24px;border-bottom:1px solid #e2e8f0}
.zyad-tenant-nav a{color:#0f172a;text-decoration:none;font-size:14px;font-weight:500}
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
)

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
	if nav := headerChromeLinks(menus); len(nav) > 0 {
		replacement := `<div data-zyad-slot="tenant-nav" class="zyad-slot">` + buildNavMarkup(nav) + `</div>`
		markup = navSentinelRe.ReplaceAllStringFunc(markup, func(string) string { return replacement })
	}

	columns := footerChromeColumns(menus)
	replacement := `<div data-zyad-slot="tenant-footer" class="zyad-slot">` +
		buildFooterMarkup(branding, columns, footerCopyright(branding, page)) + `</div>`
	markup = footerSentinelRe.ReplaceAllStringFunc(markup, func(string) string { return replacement })

	return markup
}

func buildNavMarkup(links []ssrChromeLink) string {
	var b strings.Builder
	b.WriteString(`<nav class="zyad-tenant-nav">`)
	for _, link := range links {
		writeAnchor(&b, link)
	}
	b.WriteString(`</nav>`)
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
