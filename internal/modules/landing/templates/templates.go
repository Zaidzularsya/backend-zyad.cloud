// Package templates menyimpan template halaman bawaan (seed) yang di-embed ke
// binary dan dimasukkan ke tabel landing_page_templates oleh seeder saat start.
package templates

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed seed
var seedFS embed.FS

type Seed struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Category     string `json:"category"`
	SortOrder    int    `json:"sort_order"`
	PlatformOnly bool   `json:"platform_only"`
	Version      int    `json:"version"`
	HTML         string `json:"-"`
	CSS          string `json:"-"`
}

// LoadSeeds membaca seed/<slug>/{meta.json,page.html,page.css}, urut slug.
func LoadSeeds() ([]Seed, error) {
	entries, err := fs.ReadDir(seedFS, "seed")
	if err != nil {
		return nil, err
	}
	seeds := make([]Seed, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := "seed/" + e.Name()
		rawMeta, err := seedFS.ReadFile(dir + "/meta.json")
		if err != nil {
			return nil, fmt.Errorf("seed %s: %w", e.Name(), err)
		}
		var s Seed
		if err := json.Unmarshal(rawMeta, &s); err != nil {
			return nil, fmt.Errorf("seed %s: meta.json: %w", e.Name(), err)
		}
		if s.Slug != e.Name() {
			return nil, fmt.Errorf("seed %s: slug %q tidak sama dengan nama folder", e.Name(), s.Slug)
		}
		if s.Version < 1 {
			return nil, fmt.Errorf("seed %s: version harus >= 1", e.Name())
		}
		html, err := seedFS.ReadFile(dir + "/page.html")
		if err != nil {
			return nil, fmt.Errorf("seed %s: %w", e.Name(), err)
		}
		if strings.TrimSpace(string(html)) == "" {
			return nil, fmt.Errorf("seed %s: page.html kosong", e.Name())
		}
		css, err := seedFS.ReadFile(dir + "/page.css")
		if err != nil {
			return nil, fmt.Errorf("seed %s: %w", e.Name(), err)
		}
		s.HTML, s.CSS = string(html), string(css)
		seeds = append(seeds, s)
	}
	sort.Slice(seeds, func(i, j int) bool { return seeds[i].Slug < seeds[j].Slug })
	return seeds, nil
}
