package service

import (
	"sort"
	"strings"

	"zyad.cloud/internal/modules/landing/domain"
)

// ErrMissingRequired: field wajib belum diisi. Handler memetakannya ke 422 dengan data.missing_fields.
type ErrMissingRequired struct{ Keys []string }

func (e *ErrMissingRequired) Error() string {
	return "missing required fields: " + strings.Join(e.Keys, ",")
}

// prepareSubmittedData menerapkan aturan submit server (spec §4.3) sebelum idempotency dan simpan:
// buang key asing, isi default field hidden, lalu cek field wajib. Form tanpa definisi field
// (form lama) dikembalikan apa adanya.
func prepareSubmittedData(fields []domain.LandingFormField, data map[string]any) (map[string]any, error) {
	if len(fields) == 0 {
		return data, nil
	}

	ordered := append([]domain.LandingFormField(nil), fields...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].SortOrder < ordered[j].SortOrder })

	known := make(map[string]struct{}, len(ordered)+2)
	for _, f := range ordered {
		known[f.Key] = struct{}{}
	}
	known["page_url"] = struct{}{}
	known["consent"] = struct{}{}

	out := make(map[string]any, len(data))
	for k, v := range data {
		if _, ok := known[k]; ok {
			out[k] = v
		}
	}

	var missing []string
	for _, f := range ordered {
		if f.Type == domain.FormFieldTypeHidden {
			if isEmptySubmittedValue(out[f.Key]) && f.DefaultValue != "" {
				out[f.Key] = f.DefaultValue
			}
			continue
		}
		if !f.IsRequired {
			continue
		}
		value := out[f.Key]
		if f.Type == domain.FormFieldTypeCheckbox {
			if value != true {
				missing = append(missing, f.Key)
			}
			continue
		}
		if isEmptySubmittedValue(value) {
			missing = append(missing, f.Key)
		}
	}
	if len(missing) > 0 {
		return nil, &ErrMissingRequired{Keys: missing}
	}
	return out, nil
}

func isEmptySubmittedValue(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}
