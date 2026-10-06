package domain

import (
	"bytes"
	"encoding/json"
	"strings"

	"zyad.cloud/internal/shared/pricing"
)

// FeatureDef adalah definisi fitur dari registry (modul product).
type FeatureDef struct {
	Key, Name, ValueType, Unit string
	IsActive                   bool
}

// FeatureLabel menghasilkan teks tampilan sebuah fitur. visible=false berarti
// fitur tidak ditampilkan (boolean false tanpa display_label).
func FeatureLabel(def FeatureDef, value json.RawMessage, displayLabel string) (label string, visible bool) {
	if l := strings.TrimSpace(displayLabel); l != "" {
		return l, true
	}
	raw := bytes.TrimSpace(value)
	switch def.ValueType {
	case "boolean":
		if string(raw) == "true" {
			return def.Name, true
		}
		return "", false
	case "string":
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", false
		}
		return def.Name + ": " + s, true
	default: // integer, decimal
		text := string(raw)
		if def.Unit != "" {
			text += " " + def.Unit
		}
		return def.Name + ": " + text, true
	}
}

// FrequencyRank memberi urutan tampil varian: harian paling awal, kosong paling akhir.
func FrequencyRank(f pricing.Frequency) int {
	for i, v := range pricing.Frequencies() {
		if v == f {
			return i + 1
		}
	}
	return 99
}
