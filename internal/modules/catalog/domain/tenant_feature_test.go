package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestCheckTenantFeature(t *testing.T) {
	ten := 10
	boolDef := FeatureDef{Key: "crm", ValueType: "boolean", IsActive: true, TenantOfferable: true}
	intDef := FeatureDef{Key: "users", ValueType: "integer", IsActive: true, TenantOfferable: true, TenantMax: &ten}
	cases := []struct {
		name string
		def  FeatureDef
		val  string
		want error
	}{
		{"boolean true", boolDef, `true`, nil},
		{"boolean false", boolDef, `false`, nil},
		{"boolean salah tipe", boolDef, `1`, ErrFeatureInvalidValue},
		{"integer di batas", intDef, `10`, nil},
		{"integer 1", intDef, `1`, nil},
		{"integer di atas max", intDef, `11`, ErrFeatureAboveMax},
		{"integer nol", intDef, `0`, ErrFeatureInvalidValue},
		{"integer negatif", intDef, `-3`, ErrFeatureInvalidValue},
		{"integer pecahan", intDef, `5.5`, ErrFeatureInvalidValue},
		{"integer string", intDef, `"5"`, ErrFeatureInvalidValue},
		{"integer tanpa nilai", intDef, ``, ErrFeatureInvalidValue},
		{"integer tanpa tenant_max", FeatureDef{ValueType: "integer", IsActive: true, TenantOfferable: true}, `5`, ErrFeatureNotOfferable},
		{"non-offerable", FeatureDef{ValueType: "boolean", IsActive: true}, `true`, ErrFeatureNotOfferable},
		{"nonaktif", FeatureDef{ValueType: "boolean", TenantOfferable: true}, `true`, ErrFeatureNotOfferable},
		{"decimal ditolak", FeatureDef{ValueType: "decimal", IsActive: true, TenantOfferable: true}, `1.5`, ErrFeatureInvalidValue},
		{"string ditolak", FeatureDef{ValueType: "string", IsActive: true, TenantOfferable: true}, `"x"`, ErrFeatureInvalidValue},
	}
	for _, c := range cases {
		if err := CheckTenantFeature(c.def, json.RawMessage(c.val)); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}
