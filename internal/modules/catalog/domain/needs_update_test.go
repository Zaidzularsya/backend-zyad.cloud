package domain

import (
	"encoding/json"
	"testing"
)

func TestNeedsUpdate(t *testing.T) {
	ten, five := 10, 5
	defs := map[string]FeatureDef{
		"crm":   {Key: "crm", ValueType: "boolean", IsActive: true, TenantOfferable: true},
		"users": {Key: "users", ValueType: "integer", IsActive: true, TenantOfferable: true, TenantMax: &ten},
	}
	prod := func(fs ...ProductFeature) Product { return Product{Features: fs} }
	f := func(k, v string) ProductFeature { return ProductFeature{FeatureKey: k, Value: json.RawMessage(v)} }

	if NeedsUpdate(prod(f("crm", "true"), f("users", "10")), defs) {
		t.Error("offerable dalam batas harus false")
	}
	if NeedsUpdate(prod(), defs) {
		t.Error("produk tanpa fitur harus false")
	}
	lowered := map[string]FeatureDef{"users": {Key: "users", ValueType: "integer", IsActive: true, TenantOfferable: true, TenantMax: &five}}
	if !NeedsUpdate(prod(f("users", "10")), lowered) {
		t.Error("tenant_max diturunkan di bawah nilai harus true")
	}
	revoked := map[string]FeatureDef{"crm": {Key: "crm", ValueType: "boolean", IsActive: true, TenantOfferable: false}}
	if !NeedsUpdate(prod(f("crm", "true")), revoked) {
		t.Error("tenant_offerable dicabut harus true")
	}
	inactive := map[string]FeatureDef{"crm": {Key: "crm", ValueType: "boolean", IsActive: false, TenantOfferable: true}}
	if !NeedsUpdate(prod(f("crm", "true")), inactive) {
		t.Error("fitur nonaktif harus true")
	}
	if !NeedsUpdate(prod(f("ghost", "true")), defs) {
		t.Error("definisi fitur hilang harus true")
	}
}
