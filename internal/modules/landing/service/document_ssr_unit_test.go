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
	// Header filled: brand + nav (sorted, disabled item dropped) + default action.
	if !strings.Contains(doc, `class="zyad-tenant-header zyad-tenant-header--solid zyad-tenant-header--left zyad-tenant-header--sticky"`) {
		t.Fatalf("tenant-header not filled with default presentation: %s", doc)
	}
	if !strings.Contains(doc, `<nav class="zyad-tenant-header__nav"><a href="/">Beranda</a><a href="/pricing">Harga</a></nav>`) {
		t.Fatalf("nav links wrong/unsorted: %s", doc)
	}
	if !strings.Contains(doc, `<a class="zyad-tenant-header__brand" href="/"><img src="https://cdn.test/logo.png" alt="Acme"><span>Acme</span></a>`) {
		t.Fatalf("header brand not rendered: %s", doc)
	}
	if !strings.Contains(doc, `<a class="zyad-tenant-header__action" href="/login">Masuk</a>`) {
		t.Fatalf("header action not rendered: %s", doc)
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

func TestRenderGrapesDocumentAppliesHeaderPresentation(t *testing.T) {
	page := grapesResolvedFixture()
	// bluemonday-style entity-encoded attribute value.
	page.HTML = `<div data-zyad-slot="tenant-nav" data-zyad-header="{&#34;sticky&#34;:false,&#34;variant&#34;:&#34;transparent&#34;,&#34;align&#34;:&#34;center&#34;,&#34;showAction&#34;:false,&#34;actionLabel&#34;:&#34;Masuk&#34;,&#34;actionUrl&#34;:&#34;/login&#34;}"></div><main>x</main>`

	doc := RenderGrapesDocument(page)

	// The class attribute (not the CSS block) reflects the presentation.
	if !strings.Contains(doc, `class="zyad-tenant-header zyad-tenant-header--transparent zyad-tenant-header--center"`) {
		t.Fatalf("presentation not applied (variant/align/no-sticky): %s", doc)
	}
	if strings.Contains(doc, `<a class="zyad-tenant-header__action"`) {
		t.Fatalf("showAction:false must drop the action button: %s", doc)
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

func TestRenderGrapesDocumentDropsDuplicateSentinels(t *testing.T) {
	// A page mistakenly ending up with two "Header tenant" / "Footer tenant"
	// blocks (a builder bug the GrapesEditor.vue guard now prevents going
	// forward) must self-heal at render time: only the first of each is
	// filled, the rest are dropped rather than rendering twice.
	page := grapesResolvedFixture()
	page.HTML = `<div data-zyad-slot="tenant-nav"><span>placeholder</span></div>` +
		`<main><h1>Konten</h1></main>` +
		`<div data-zyad-slot="tenant-nav"><span>placeholder</span></div>` +
		`<div data-zyad-slot="tenant-footer"><span>placeholder</span></div>` +
		`<div data-zyad-slot="tenant-footer"><span>placeholder</span></div>`

	doc := RenderGrapesDocument(page)

	if n := strings.Count(doc, `class="zyad-tenant-header `); n != 1 {
		t.Fatalf("expected exactly 1 rendered header, got %d: %s", n, doc)
	}
	if n := strings.Count(doc, `class="zyad-tenant-footer"`); n != 1 {
		t.Fatalf("expected exactly 1 rendered footer, got %d: %s", n, doc)
	}
	if n := strings.Count(doc, `data-zyad-slot="tenant-nav"`); n != 1 {
		t.Fatalf("expected exactly 1 remaining tenant-nav sentinel, got %d: %s", n, doc)
	}
	if n := strings.Count(doc, `data-zyad-slot="tenant-footer"`); n != 1 {
		t.Fatalf("expected exactly 1 remaining tenant-footer sentinel, got %d: %s", n, doc)
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
