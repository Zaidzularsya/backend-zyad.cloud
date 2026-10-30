package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	return s.validateFeaturesWith(ctx, features, nil)
}

// validateTenantFeatures: validasi dasar yang sama, ditambah batas katalog
// (CheckTenantFeature) untuk org non-platform.
func (s *productService) validateTenantFeatures(ctx context.Context, features []repository.FeatureValue) error {
	return s.validateFeaturesWith(ctx, features, tenantFeatureCheck)
}

func tenantFeatureCheck(def domain.FeatureDef, value json.RawMessage) error {
	switch err := domain.CheckTenantFeature(def, value); {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrFeatureNotOfferable):
		return ErrFeatureNotOfferable
	case errors.Is(err, domain.ErrFeatureAboveMax):
		return ErrFeatureAboveMax
	default:
		return ErrInvalidFeature
	}
}

func (s *productService) validateFeaturesWith(ctx context.Context, features []repository.FeatureValue, check func(domain.FeatureDef, json.RawMessage) error) error {
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
		if check != nil {
			if err := check(def, f.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

// fillFeatureLabels mengisi Features[i].Label dengan satu panggilan registry. Key
// nonaktif tetap diberi label supaya produk lama tetap terbaca.
func fillFeatureLabels(ctx context.Context, registry FeatureRegistry, products []domain.Product) error {
	return fillFeatureInfo(ctx, registry, products, false)
}

// fillFeatureInfo: label + (bila flagNeedsUpdate) Product.NeedsUpdate, dari satu
// panggilan FindByKeys. flagNeedsUpdate hanya untuk produk org non-platform.
func fillFeatureInfo(ctx context.Context, registry FeatureRegistry, products []domain.Product, flagNeedsUpdate bool) error {
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
		if flagNeedsUpdate {
			products[i].NeedsUpdate = domain.NeedsUpdate(products[i], defs)
		}
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

// ExcludeNeedsUpdate membuang produk non-platform yang perlu diperbarui (satu
// panggilan FindByKeys). Dipakai listing publik org non-platform dan checkout.
func ExcludeNeedsUpdate(ctx context.Context, registry FeatureRegistry, products []domain.Product) ([]domain.Product, error) {
	if err := fillFeatureInfo(ctx, registry, products, true); err != nil {
		return nil, err
	}
	out := make([]domain.Product, 0, len(products))
	for _, p := range products {
		if !p.NeedsUpdate {
			out = append(out, p)
		}
	}
	return out, nil
}
