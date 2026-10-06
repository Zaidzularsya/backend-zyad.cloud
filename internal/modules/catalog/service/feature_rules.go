package service

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/catalog/repository"
)

const (
	maxProductFeatures = 30
	maxFeatureLabel    = 200
	maxFeatureString   = 200
)

var listingCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,49}$`)

// ValidateFeatureValue memeriksa nilai JSON terhadap value_type registry.
func ValidateFeatureValue(def domain.FeatureDef, value json.RawMessage) bool {
	raw := bytes.TrimSpace(value)
	if len(raw) == 0 {
		return false
	}
	switch def.ValueType {
	case "boolean":
		return string(raw) == "true" || string(raw) == "false"
	case "integer", "decimal":
		if raw[0] == '"' {
			return false
		}
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
			return false
		}
		return def.ValueType == "decimal" || n == math.Trunc(n) && !bytes.ContainsAny(raw, ".eE")
	case "string":
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return false
		}
		n := utf8.RuneCountInString(s)
		return n >= 1 && n <= maxFeatureString
	}
	return false
}

func validListingCode(code string) bool { return listingCodePattern.MatchString(code) }

// validateListing memeriksa blok publikasi terhadap kategori efektif produk.
func validateListing(l repository.ListingParams, categoryID string) error {
	code := strings.TrimSpace(l.ListingCode)
	if code != "" && !validListingCode(code) {
		return ErrInvalidListing
	}
	if l.IsPublic && (code == "" || strings.TrimSpace(categoryID) == "") {
		return ErrInvalidListing
	}
	return nil
}

func (s *productService) validateFeatures(ctx context.Context, features []repository.FeatureValue) error {
	if len(features) > maxProductFeatures {
		return ErrInvalidFeature
	}
	if len(features) == 0 {
		return nil
	}
	keys := make([]string, 0, len(features))
	seen := make(map[string]bool, len(features))
	for _, f := range features {
		if f.FeatureKey == "" || seen[f.FeatureKey] || utf8.RuneCountInString(strings.TrimSpace(f.DisplayLabel)) > maxFeatureLabel {
			return ErrInvalidFeature
		}
		seen[f.FeatureKey] = true
		keys = append(keys, f.FeatureKey)
	}
	defs, err := s.features.FindByKeys(ctx, keys)
	if err != nil {
		return err
	}
	for _, f := range features {
		def, ok := defs[f.FeatureKey]
		if !ok || !def.IsActive || !ValidateFeatureValue(def, f.Value) {
			return ErrInvalidFeature
		}
	}
	return nil
}

// fillFeatureLabels mengisi Features[i].Label dengan satu panggilan registry. Key
// nonaktif tetap diberi label supaya produk lama tetap terbaca.
func fillFeatureLabels(ctx context.Context, registry FeatureRegistry, products []domain.Product) error {
	var keys []string
	seen := map[string]bool{}
	for _, p := range products {
		for _, f := range p.Features {
			if !seen[f.FeatureKey] {
				seen[f.FeatureKey] = true
				keys = append(keys, f.FeatureKey)
			}
		}
	}
	if len(keys) == 0 {
		return nil
	}
	defs, err := registry.FindByKeys(ctx, keys)
	if err != nil {
		return err
	}
	for i := range products {
		for j := range products[i].Features {
			f := &products[i].Features[j]
			def, ok := defs[f.FeatureKey]
			if !ok {
				def = domain.FeatureDef{Key: f.FeatureKey, Name: f.FeatureKey}
			}
			label, visible := domain.FeatureLabel(def, f.Value, f.DisplayLabel)
			if !visible {
				label = ""
			}
			f.Label = label
		}
	}
	return nil
}
