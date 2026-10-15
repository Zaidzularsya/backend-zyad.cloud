package service

import (
	"strings"
	"testing"
)

func TestLockPlatformOnly(t *testing.T) {
	slot := `<div data-zyad-slot="catalog-pricing"></div>`
	if !LockPlatformOnly(slot, false) {
		t.Error("catalog-pricing slot must force platform_only")
	}
	if LockPlatformOnly(`<p>x</p>`, false) {
		t.Error("plain html with requested=false must stay false")
	}
	if !LockPlatformOnly(`<p>x</p>`, true) {
		t.Error("requested=true must stay true")
	}
}

func TestIsEmptyTemplateMarkup(t *testing.T) {
	cases := []struct {
		name string
		html string
		want bool
	}{
		{"empty", "", true},
		{"whitespace", "  \n ", true},
		{"only sentinels", `<div data-zyad-slot="tenant-nav"></div><div data-zyad-slot="tenant-footer"></div>`, true},
		{"has section", `<section><h2>A</h2></section>`, false},
		{"sentinel plus section", `<div data-zyad-slot="tenant-nav"></div><section>A</section>`, false},
	}
	for _, c := range cases {
		if got := IsEmptyTemplateMarkup(c.html); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestValidatePageTemplateMeta(t *testing.T) {
	ok := func(name, slug, cat, desc string, sort int) error {
		return ValidatePageTemplateMeta(name, slug, cat, desc, sort)
	}
	if err := ok("Klinik", "klinik", "kesehatan", "desc", 10); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	bad := map[string]error{
		"slug uppercase": ok("N", "Klinik", "kesehatan", "", 1),
		"slug empty":     ok("N", "", "kesehatan", "", 1),
		"slug 81":        ok("N", strings.Repeat("a", 81), "kesehatan", "", 1),
		"name empty":     ok("   ", "a", "kesehatan", "", 1),
		"name 121":       ok(strings.Repeat("n", 121), "a", "kesehatan", "", 1),
		"desc 241":       ok("N", "a", "kesehatan", strings.Repeat("d", 241), 1),
		"category":       ok("N", "a", "lainnya", "", 1),
		"sort -1":        ok("N", "a", "kesehatan", "", -1),
		"sort 10000":     ok("N", "a", "kesehatan", "", 10000),
	}
	for name, err := range bad {
		if err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestNextWorkPageSlug(t *testing.T) {
	taken := map[string]bool{}
	f := func(s string) bool { return taken[s] }
	if got := NextWorkPageSlug("klinik", f); got != "template-klinik" {
		t.Errorf("got %q", got)
	}
	taken["template-klinik"] = true
	if got := NextWorkPageSlug("klinik", f); got != "template-klinik-2" {
		t.Errorf("got %q", got)
	}
	taken["template-klinik-2"] = true
	if got := NextWorkPageSlug("klinik", f); got != "template-klinik-3" {
		t.Errorf("got %q", got)
	}
}
