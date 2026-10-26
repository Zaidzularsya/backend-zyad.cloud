package app

import (
	"bytes"
	"encoding/json"
	"strconv"

	crmdomain "zyad.cloud/internal/modules/crm/domain"
)

// featurePageActivation adalah kemampuan reseller (spec tenant-products §5.2): menjual halaman ke
// kliennya lewat invoice aktivasi. Fitur ini sendiri tidak cukup untuk membuat halaman, jadi
// workspace yang menerimanya juga harus mendapat bundle di bawah.
const featurePageActivation = "landing.page_activation"

const (
	resellerMaxPages           = 10
	resellerMaxSectionsPerPage = 12
	resellerStorageBytes       = 200 * 1024 * 1024
)

// resellerBundleSpec adalah fitur yang menyertai landing.page_activation. landing.max_pages wajib ada:
// pembuatan halaman gagal keras bila barisnya tidak ada. storage.max_bytes wajib ada: tanpa baris
// itu upload dianggap tanpa batas. ValueType mengikuti registry fitur.
var resellerBundleSpec = []struct{ Key, ValueType, Value string }{
	{"landing.enabled", "boolean", "true"},
	{"landing.max_pages", "integer", strconv.Itoa(resellerMaxPages)},
	{"landing.max_sections_per_page", "integer", strconv.Itoa(resellerMaxSectionsPerPage)},
	{"storage.max_bytes", "integer", strconv.Itoa(resellerStorageBytes)},
}

func resellerBundle() []crmdomain.FeatureSnapshot {
	out := make([]crmdomain.FeatureSnapshot, len(resellerBundleSpec))
	for i, f := range resellerBundleSpec {
		out[i] = crmdomain.FeatureSnapshot{FeatureKey: f.Key, Value: json.RawMessage(f.Value)}
	}
	return out
}

// withResellerBundle melengkapi fitur contract yang membawa landing.page_activation=true dengan
// bundle reseller. Nilai yang sudah ditentukan produk tidak ditimpa. Masukan tidak diubah.
func withResellerBundle(features []crmdomain.FeatureSnapshot) []crmdomain.FeatureSnapshot {
	activated := false
	present := make(map[string]bool, len(features))
	for _, f := range features {
		present[f.FeatureKey] = true
		if f.FeatureKey == featurePageActivation && bytes.Equal(bytes.TrimSpace(f.Value), []byte("true")) {
			activated = true
		}
	}
	if !activated {
		return features
	}
	out := append([]crmdomain.FeatureSnapshot(nil), features...)
	for _, implied := range resellerBundle() {
		if !present[implied.FeatureKey] {
			out = append(out, implied)
		}
	}
	return out
}
