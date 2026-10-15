package templates

import (
	"testing"

	"zyad.cloud/internal/modules/landing/domain"
)

func TestLoadSeeds_ValidAndComplete(t *testing.T) {
	seeds, err := LoadSeeds()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"asosiasi", "asosiasi-foto", "company-profile", "event", "isp-foto", "isp-rtrw-net",
		"jasa-foto", "jasa-profesional", "klinik", "klinik-foto", "portfolio", "pricing",
		"properti", "properti-foto", "restoran-foto", "restoran-kafe", "saas",
		"sekolah-foto", "sekolah-kursus", "toko-foto", "toko-umkm", "zyad-marketing",
	}
	if len(seeds) != len(want) {
		t.Fatalf("got %d seeds, want %d", len(seeds), len(want))
	}
	for i, s := range seeds {
		if s.Slug != want[i] {
			t.Errorf("seed[%d] slug = %q, want %q (sorted by slug)", i, s.Slug, want[i])
		}
		if !domain.IsValidPageTemplateCategory(s.Category) {
			t.Errorf("%s: invalid category %q", s.Slug, s.Category)
		}
		if s.Version < 1 || s.HTML == "" || s.Name == "" {
			t.Errorf("%s: incomplete seed", s.Slug)
		}
		if s.Slug == "zyad-marketing" != s.PlatformOnly {
			t.Errorf("%s: platform_only = %v", s.Slug, s.PlatformOnly)
		}
	}
}
