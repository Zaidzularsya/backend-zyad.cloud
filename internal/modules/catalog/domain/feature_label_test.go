package domain

import (
	"encoding/json"
	"testing"

	"zyad.cloud/internal/shared/pricing"
)

func TestFeatureLabel(t *testing.T) {
	cases := []struct {
		name    string
		def     FeatureDef
		value   string
		display string
		want    string
		visible bool
	}{
		{"boolean true", FeatureDef{Name: "CRM", ValueType: "boolean"}, `true`, "", "CRM", true},
		{"boolean false", FeatureDef{Name: "CRM", ValueType: "boolean"}, `false`, "", "", false},
		{"integer dengan unit", FeatureDef{Name: "Jumlah user", ValueType: "integer", Unit: "user"}, `5`, "", "Jumlah user: 5 user", true},
		{"decimal tanpa unit", FeatureDef{Name: "Storage", ValueType: "decimal"}, `2.5`, "", "Storage: 2.5", true},
		{"string", FeatureDef{Name: "Support", ValueType: "string"}, `"Prioritas"`, "", "Support: Prioritas", true},
		{"display_label menimpa", FeatureDef{Name: "Jumlah user", ValueType: "integer", Unit: "user"}, `5`, "Hingga 5 user", "Hingga 5 user", true},
		{"display_label pada boolean false", FeatureDef{Name: "CRM", ValueType: "boolean"}, `false`, "Tanpa CRM", "Tanpa CRM", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, visible := FeatureLabel(c.def, json.RawMessage(c.value), c.display)
			if got != c.want || visible != c.visible {
				t.Fatalf("got (%q,%v), want (%q,%v)", got, visible, c.want, c.visible)
			}
		})
	}
}

func TestFrequencyRank(t *testing.T) {
	if !(FrequencyRank(pricing.Monthly) < FrequencyRank(pricing.Quarterly) && FrequencyRank(pricing.Quarterly) < FrequencyRank(pricing.Annual)) {
		t.Fatal("urutan frekuensi salah")
	}
	if FrequencyRank("") != 99 || FrequencyRank("") <= FrequencyRank(pricing.Annual) {
		t.Fatal("kosong harus paling akhir")
	}
}
