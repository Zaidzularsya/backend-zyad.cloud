package app

import (
	"encoding/json"
	"testing"

	crmdomain "zyad.cloud/internal/modules/crm/domain"
)

func snap(key, value string) crmdomain.FeatureSnapshot {
	return crmdomain.FeatureSnapshot{FeatureKey: key, Value: json.RawMessage(value)}
}

func valuesByKey(fs []crmdomain.FeatureSnapshot) map[string]string {
	out := map[string]string{}
	for _, f := range fs {
		out[f.FeatureKey] = string(f.Value)
	}
	return out
}

func TestWithResellerBundleAddsImpliedFeatures(t *testing.T) {
	got := valuesByKey(withResellerBundle([]crmdomain.FeatureSnapshot{snap("landing.page_activation", "true")}))
	want := map[string]string{
		"landing.page_activation":       "true",
		"landing.enabled":               "true",
		"landing.max_pages":             "10",
		"landing.max_sections_per_page": "12",
		"storage.max_bytes":             "209715200",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
}

func TestWithResellerBundleKeepsExplicitValues(t *testing.T) {
	got := valuesByKey(withResellerBundle([]crmdomain.FeatureSnapshot{
		snap("landing.page_activation", "true"),
		snap("landing.max_pages", "25"),
		snap("storage.max_bytes", "1073741824"),
	}))
	if got["landing.max_pages"] != "25" || got["storage.max_bytes"] != "1073741824" {
		t.Fatalf("nilai eksplisit produk tidak boleh ditimpa: %v", got)
	}
	if got["landing.enabled"] != "true" || got["landing.max_sections_per_page"] != "12" {
		t.Fatalf("fitur yang belum ada harus dilengkapi: %v", got)
	}
}

func TestWithResellerBundleNoopWithoutActivation(t *testing.T) {
	in := []crmdomain.FeatureSnapshot{snap("crm.enabled", "true"), snap("landing.page_activation", "false")}
	if out := withResellerBundle(in); len(out) != len(in) {
		t.Fatalf("tanpa page_activation=true tidak boleh menambah fitur: %v", valuesByKey(out))
	}
	if out := withResellerBundle(nil); len(out) != 0 {
		t.Fatalf("nil harus tetap kosong: %v", out)
	}
}

func TestWithResellerBundleIsIdempotentAndDoesNotMutateInput(t *testing.T) {
	in := []crmdomain.FeatureSnapshot{snap("landing.page_activation", "true")}
	once := withResellerBundle(in)
	twice := withResellerBundle(once)
	if len(once) != len(twice) {
		t.Fatalf("bundle dua kali harus sama: %d vs %d", len(once), len(twice))
	}
	if len(in) != 1 {
		t.Fatalf("slice masukan tidak boleh berubah: %v", in)
	}
}
