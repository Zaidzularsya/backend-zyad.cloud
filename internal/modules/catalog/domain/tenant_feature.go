package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
)

var (
	ErrFeatureNotOfferable = errors.New("fitur tidak boleh ditawarkan tenant")
	ErrFeatureAboveMax     = errors.New("nilai fitur melebihi batas katalog")
	ErrFeatureInvalidValue = errors.New("nilai fitur tidak valid untuk tenant")
)

// CheckTenantFeature adalah aturan tunggal batas katalog untuk fitur produk
// tenant: fitur aktif dan tenant_offerable; boolean true/false; integer 1..tenant_max.
// Tipe decimal/string tidak boleh dijual tenant. Murni (tanpa I/O) agar dapat
// dipakai ulang oleh pemeriksaan "perlu pembaruan".
func CheckTenantFeature(def FeatureDef, value json.RawMessage) error {
	if !def.IsActive || !def.TenantOfferable {
		return ErrFeatureNotOfferable
	}
	raw := bytes.TrimSpace(value)
	switch def.ValueType {
	case "boolean":
		if string(raw) == "true" || string(raw) == "false" {
			return nil
		}
		return ErrFeatureInvalidValue
	case "integer":
		if def.TenantMax == nil || *def.TenantMax < 1 {
			return ErrFeatureNotOfferable
		}
		if len(raw) == 0 || raw[0] == '"' || bytes.ContainsAny(raw, ".eE") {
			return ErrFeatureInvalidValue
		}
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil || math.IsInf(n, 0) || math.IsNaN(n) || n < 1 {
			return ErrFeatureInvalidValue
		}
		if n > float64(*def.TenantMax) {
			return ErrFeatureAboveMax
		}
		return nil
	}
	return ErrFeatureInvalidValue
}

// NeedsUpdate: produk tenant "perlu diperbarui" bila ada fitur yang definisinya
// hilang atau tidak lagi lolos CheckTenantFeature (nonaktif, tidak tenant_offerable,
// atau melebihi tenant_max). Diturunkan saat dibaca; org platform dikecualikan
// oleh pemanggil. Tidak mengubah kontrak/entitlement yang sudah terjual.
func NeedsUpdate(p Product, defs map[string]FeatureDef) bool {
	for _, f := range p.Features {
		def, ok := defs[f.FeatureKey]
		if !ok || CheckTenantFeature(def, f.Value) != nil {
			return true
		}
	}
	return false
}
