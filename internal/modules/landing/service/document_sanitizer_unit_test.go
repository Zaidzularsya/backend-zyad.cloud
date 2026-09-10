package service

import (
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/landing/domain"
)

func TestSanitizeGrapesHTMLStripsExecutableConstructs(t *testing.T) {
	in := `
		<section class="hero" id="top" data-zyad-slot="tenant-nav">
			<script>alert('xss')</script>
			<h1 onclick="steal()">Judul</h1>
			<a href="javascript:alert(1)">bad</a>
			<a href="https://ok.test" target="_blank">ok</a>
			<iframe src="https://evil.test"></iframe>
			<img src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7" alt="ok">
			<div style="color:red;background:url(javascript:alert(1))">styled</div>
		</section>`

	out := SanitizeGrapesHTML(in)

	for _, banned := range []string{"<script", "onclick", "<iframe", "javascript:alert(1)"} {
		if strings.Contains(out, banned) {
			t.Fatalf("sanitized HTML still contains %q:\n%s", banned, out)
		}
	}
	for _, kept := range []string{`class="hero"`, `id="top"`, `data-zyad-slot="tenant-nav"`, `href="https://ok.test"`, `data:image/gif;base64,R0lGOD`, "color:red"} {
		if !strings.Contains(out, kept) {
			t.Fatalf("sanitized HTML dropped expected %q:\n%s", kept, out)
		}
	}
	// the dangerous url() inside an inline style must be neutralised, not kept
	if strings.Contains(out, "url(javascript") {
		t.Fatalf("inline style url(javascript:) survived:\n%s", out)
	}
}

func TestSanitizeGrapesCSS(t *testing.T) {
	in := `
		@import url('https://evil.test/x.css');
		.a { color: #333; background: url(https://cdn.test/bg.png); }
		.b { background: url(javascript:alert(1)); }
		.c { width: expression(alert(1)); }
		.d { behavior: url(x.htc); }`

	out := SanitizeGrapesCSS(in)

	for _, banned := range []string{"@import", "expression(", "behavior:", "url(javascript"} {
		if strings.Contains(strings.ToLower(out), banned) {
			t.Fatalf("sanitized CSS still contains %q:\n%s", banned, out)
		}
	}
	if !strings.Contains(out, "url(https://cdn.test/bg.png)") {
		t.Fatalf("sanitized CSS dropped a safe url():\n%s", out)
	}
	if !strings.Contains(out, "color: #333") {
		t.Fatalf("sanitized CSS dropped a plain declaration:\n%s", out)
	}
}

func TestBuildGrapesJSSnapshotSanitizesAndTags(t *testing.T) {
	page := domain.LandingPage{ID: "p1", Slug: "home", SEO: map[string]any{"title": "Home"}}
	doc := domain.LandingPageDocument{
		HTML: `<div><script>x</script><p>hi</p></div>`,
		CSS:  `@import url(x); p{color:red}`,
	}

	snap := buildGrapesJSSnapshot(page, doc, time.Unix(0, 0).UTC())

	if snap["builder"] != string(domain.PageBuilderGrapesJS) {
		t.Fatalf("builder tag = %v", snap["builder"])
	}
	html, _ := snap["html"].(string)
	css, _ := snap["css"].(string)
	if strings.Contains(html, "<script") || !strings.Contains(html, "<p>hi</p>") {
		t.Fatalf("snapshot html not sanitized: %q", html)
	}
	if strings.Contains(strings.ToLower(css), "@import") || !strings.Contains(css, "color:red") {
		t.Fatalf("snapshot css not sanitized: %q", css)
	}
	if _, ok := snap["project"].(map[string]any); !ok {
		t.Fatalf("snapshot project should default to an object, got %T", snap["project"])
	}

	gotHTML, gotCSS := grapesSnapshotMarkup(snap)
	if gotHTML != html || gotCSS != css {
		t.Fatalf("grapesSnapshotMarkup mismatch")
	}
	if h, c := grapesSnapshotMarkup(nil); h != "" || c != "" {
		t.Fatalf("grapesSnapshotMarkup(nil) should be empty")
	}
}
