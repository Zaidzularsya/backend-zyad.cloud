package service

import (
	"strings"
	"testing"

	"zyad.cloud/internal/modules/landing/domain"
)

func grapesResolvedFixture() ResolvedPage {
	return ResolvedPage{
		Builder: string(domain.PageBuilderGrapesJS),
		Page: domain.LandingPage{
			Title:      "Halaman Demo",
			Visibility: domain.PageVisibilityPublic,
			SEO: map[string]any{
				"meta_title":       "Judul SEO",
				"meta_description": "Deskripsi SEO halaman.",
				"open_graph":       map[string]any{"image_url": "https://cdn.test/og.png"},
			},
		},
		Branding: domain.LandingBranding{CompanyName: "Acme", LogoLightURL: "https://cdn.test/logo.png"},
		HTML: `<div data-zyad-slot="tenant-nav"><span style="border:1px dashed red">placeholder</span></div>` +
			`<main><h1>Konten</h1></main>` +
			`<div data-zyad-slot="tenant-footer"><span>placeholder</span></div>`,
		CSS: "h1{color:#111}",
		Menus: []ResolvedMenu{
			{
				Location: "header",
				IsActive: true,
				Items: []ResolvedMenuItem{
					{Label: "Harga", LinkType: "internal_page", Destination: "pricing", SortOrder: 2, IsEnabled: true},
					{Label: "Beranda", LinkType: "internal_page", Destination: "public-marketing", SortOrder: 1, IsEnabled: true},
					{Label: "Off", LinkType: "anchor", Destination: "x", SortOrder: 3, IsEnabled: false},
				},
			},
			{
				Name:     "Produk",
				Location: "footer",
				IsActive: true,
				Items: []ResolvedMenuItem{
					{Label: "Fitur", LinkType: "internal_page", Destination: "fitur", SortOrder: 1, IsEnabled: true},
				},
			},
		},
	}
}

func TestRenderGrapesDocumentBuildsFullPage(t *testing.T) {
	doc := RenderGrapesDocument(grapesResolvedFixture())

	for _, want := range []string{
		"<!doctype html>",
		"<title>Judul SEO</title>",
		`<meta name="description" content="Deskripsi SEO halaman.">`,
		`<meta name="robots" content="index, follow">`,
		`<meta property="og:image" content="https://cdn.test/og.png">`,
		"h1{color:#111}",
		"<h1>Konten</h1>",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("rendered document missing %q\n---\n%s", want, doc)
		}
	}

	if strings.Contains(doc, "<script") {
		t.Fatalf("rendered document must not contain <script>: %s", doc)
	}
}

func TestRenderGrapesDocumentFillsTenantSentinels(t *testing.T) {
	doc := RenderGrapesDocument(grapesResolvedFixture())

	if strings.Contains(doc, "placeholder") || strings.Contains(doc, "dashed red") {
		t.Fatalf("sentinel placeholder not replaced: %s", doc)
	}
	// Header nav filled, sorted by sort_order, disabled item dropped.
	navIdx := strings.Index(doc, `<nav class="zyad-tenant-nav">`)
	if navIdx < 0 {
		t.Fatalf("tenant-nav not filled: %s", doc)
	}
	if !strings.Contains(doc, `<a href="/">Beranda</a><a href="/pricing">Harga</a>`) {
		t.Fatalf("nav links wrong/unsorted: %s", doc)
	}
	if strings.Contains(doc, ">Off<") {
		t.Fatalf("disabled nav item leaked: %s", doc)
	}
	// Footer filled with brand + column + copyright.
	if !strings.Contains(doc, `<span>Acme</span>`) ||
		!strings.Contains(doc, `<p class="zyad-tenant-footer__title">Produk</p>`) ||
		!strings.Contains(doc, `<a href="/fitur">Fitur</a>`) {
		t.Fatalf("footer chrome not filled: %s", doc)
	}
	if !strings.Contains(doc, `class="zyad-tenant-footer__copyright"`) {
		t.Fatalf("footer copyright missing: %s", doc)
	}
}

func TestRenderGrapesDocumentSanitisesChromeHrefsAndLabels(t *testing.T) {
	page := grapesResolvedFixture()
	page.Menus[0].Items = []ResolvedMenuItem{
		{Label: `<script>x</script>`, LinkType: "external_link", Destination: "javascript:alert(1)", IsEnabled: true},
	}

	doc := RenderGrapesDocument(page)

	if strings.Contains(doc, "javascript:alert(1)") {
		t.Fatalf("unsafe href not neutralised: %s", doc)
	}
	if !strings.Contains(doc, `<a href="#">`) {
		t.Fatalf("expected unsafe href to become #: %s", doc)
	}
	if strings.Contains(doc, "<script>x</script>") {
		t.Fatalf("nav label not escaped: %s", doc)
	}
	if !strings.Contains(doc, "&lt;script&gt;x&lt;/script&gt;") {
		t.Fatalf("expected escaped label: %s", doc)
	}
}

func TestRenderGrapesDocumentEmptyForNonGrapesPage(t *testing.T) {
	if got := RenderGrapesDocument(ResolvedPage{Page: domain.LandingPage{Title: "x"}}); got != "" {
		t.Fatalf("expected empty string for a page with no HTML, got %q", got)
	}
}

func TestRenderGrapesDocumentRobotsNoindexForPrivate(t *testing.T) {
	page := grapesResolvedFixture()
	page.Page.Visibility = domain.PageVisibilityPrivate

	doc := RenderGrapesDocument(page)
	if !strings.Contains(doc, `<meta name="robots" content="noindex, nofollow">`) {
		t.Fatalf("private page must be noindex: %s", doc)
	}
}
