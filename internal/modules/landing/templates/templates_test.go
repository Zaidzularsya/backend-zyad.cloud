package templates

import (
	"encoding/json"
	"regexp"
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

// leadFormStyles = pemetaan gaya Form Konsultasi per template (spec Form Konsultasi Fleksibel §7).
var leadFormStyles = map[string]string{
	"klinik": "minimal", "klinik-foto": "glass",
	"sekolah-kursus": "kartu", "sekolah-foto": "glass",
	"asosiasi": "kartu", "asosiasi-foto": "glass",
	"isp-rtrw-net": "inline", "isp-foto": "glass",
	"jasa-profesional": "split", "jasa-foto": "split",
	"toko-umkm": "inline", "toko-foto": "kartu",
	"restoran-kafe": "minimal", "restoran-foto": "glass",
	"properti": "minimal", "properti-foto": "glass",
	"zyad-marketing": "split",
}

var leadFormSentinel = regexp.MustCompile(`<div data-zyad-slot="lead-form" data-zyad-config='([^']*)'>`)

type seedLeadFormConfig struct {
	Style   string `json:"style"`
	Columns *int   `json:"columns"`
	Split   *struct {
		Title  string   `json:"title"`
		Points []string `json:"points"`
	} `json:"split"`
}

func TestSeeds_LeadFormStyles(t *testing.T) {
	seeds, err := LoadSeeds()
	if err != nil {
		t.Fatal(err)
	}
	jasaSplit := splitWant{"Konsultasi gratis 30 menit", []string{
		"Tanpa biaya dan tanpa komitmen",
		"Dijawab tim kami dalam 1 hari kerja",
		"Rekomendasi sesuai kebutuhan Anda",
	}}
	splits := map[string]splitWant{
		"jasa-profesional": jasaSplit,
		"jasa-foto":        jasaSplit,
		"zyad-marketing": {"Bicarakan kebutuhan bisnis Anda", []string{
			"Demo 30 menit bersama tim kami",
			"Rekomendasi paket sesuai alur bisnis Anda",
			"Didukung tim lokal Indonesia",
		}},
	}
	twoColumns := map[string]bool{"sekolah-kursus": true, "asosiasi": true}

	seen := 0
	for _, s := range seeds {
		matches := leadFormSentinel.FindAllStringSubmatch(s.HTML, -1)
		wantStyle, mapped := leadFormStyles[s.Slug]
		if !mapped {
			if len(matches) != 0 {
				t.Errorf("%s: unexpected lead-form sentinel", s.Slug)
			}
			continue
		}
		seen++
		if len(matches) != 1 {
			t.Errorf("%s: want exactly one lead-form sentinel, got %d", s.Slug, len(matches))
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(matches[0][1]), &raw); err != nil {
			t.Errorf("%s: invalid data-zyad-config JSON: %v", s.Slug, err)
			continue
		}
		if _, has := raw["form_id"]; has {
			t.Errorf("%s: seed must not be bound to a form (form_id present)", s.Slug)
		}
		var cfg seedLeadFormConfig
		_ = json.Unmarshal([]byte(matches[0][1]), &cfg)
		if cfg.Style != wantStyle {
			t.Errorf("%s: style = %q, want %q", s.Slug, cfg.Style, wantStyle)
		}
		if twoColumns[s.Slug] && (cfg.Columns == nil || *cfg.Columns != 2) {
			t.Errorf("%s: want columns 2", s.Slug)
		}
		if want, ok := splits[s.Slug]; ok {
			if cfg.Split == nil || cfg.Split.Title != want.title || len(cfg.Split.Points) != len(want.points) {
				t.Errorf("%s: split = %+v, want %+v", s.Slug, cfg.Split, want)
				continue
			}
			for i, p := range want.points {
				if cfg.Split.Points[i] != p {
					t.Errorf("%s: split point %d = %q, want %q", s.Slug, i, cfg.Split.Points[i], p)
				}
			}
		} else if cfg.Split != nil {
			t.Errorf("%s: non-split style must not carry split content", s.Slug)
		}
	}
	if seen != len(leadFormStyles) {
		t.Errorf("checked %d templates, want %d", seen, len(leadFormStyles))
	}
}

type splitWant struct {
	title  string
	points []string
}
