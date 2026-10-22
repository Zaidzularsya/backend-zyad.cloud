package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func freePlanByKey(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	out := map[string]json.RawMessage{}
	for _, f := range FreePlanFeatures() {
		if _, dup := out[f.FeatureKey]; dup {
			t.Fatalf("duplicate feature key %s", f.FeatureKey)
		}
		out[f.FeatureKey] = f.Value
	}
	return out
}

func TestFreePlanFeaturesMatchSpecTable(t *testing.T) {
	want := map[string]string{
		"landing.enabled": "true", "landing.max_pages": "3", "landing.max_sections_per_page": "12", "landing.custom_domain": "false",
		"domain.enabled": "true", "domain.max_domains": "1",
		"media.enabled": "true", "media.max_storage_mb": "500", "storage.max_bytes": "524288000", "media.max_file_size_mb": "10",
		"users.max_users": "2", "users.invite_user": "true",
		"crm.enabled": "true", "crm.pipeline": "true", "crm.lead_form": "true",
		"crm.max_leads": "50", "crm.max_contacts": "100", "crm.max_companies": "50", "crm.max_deals": "25", "crm.max_pipelines": "1",
		"crm.max_quotations_per_month": "10", "crm.max_sales_orders_per_month": "10",
		"receivable.enabled": "true", "receivable.max_invoices_per_month": "10",
	}
	got := freePlanByKey(t)
	for key, value := range want {
		raw, ok := got[key]
		if !ok {
			t.Errorf("missing key %s", key)
			continue
		}
		if string(raw) != value {
			t.Errorf("%s = %s, want %s", key, raw, value)
		}
	}
	for key := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("unexpected key %s in Free plan", key)
		}
	}
	for key := range got {
		for _, forbidden := range []string{"payment", "credit_note", "refund", "catalog_product"} {
			if strings.Contains(key, forbidden) {
				t.Errorf("Free plan must not carry quota key %s", key)
			}
		}
	}
}

// registryFromMigrations membaca feature_key -> value_type dari migration yang mendaftarkan registry.
func registryFromMigrations(t *testing.T) map[string]string {
	t.Helper()
	row := regexp.MustCompile(`\(\s*'([a-z][a-z0-9_]*(?:\.[a-z0-9_]+)+)',\s*'[a-z_]+',\s*'(?:[^']|'')*',\s*'(?:[^']|'')*',\s*'(boolean|integer|decimal|string)'`)
	out := map[string]string{}
	for _, file := range []string{
		"000055_seed_billing_features_plans", "000061_seed_product_catalog", "000074_seed_crm_lead_form_feature",
		"000128_seed_whatsapp_max_sessions_feature", "000144_create_receivable", "000158_entitlement_quota_features",
		"000159_domain_max_domains_feature", "000160_storage_max_bytes_feature",
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", "migrations", file+".up.sql"))
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		for _, m := range row.FindAllStringSubmatch(string(data), -1) {
			out[m[1]] = m[2]
		}
	}
	return out
}

func TestFreePlanFeaturesAllExistInRegistryMigrations(t *testing.T) {
	registry := registryFromMigrations(t)
	if len(registry) < 30 {
		t.Fatalf("registry parse looks broken: %d keys", len(registry))
	}
	for _, f := range FreePlanFeatures() {
		if _, ok := registry[f.FeatureKey]; !ok {
			t.Errorf("Free plan key %s is not registered by any registry migration", f.FeatureKey)
		}
	}
}

func TestFreePlanFeatureValuesMatchRegistryValueTypes(t *testing.T) {
	registry := registryFromMigrations(t)
	for _, f := range FreePlanFeatures() {
		var v any
		if err := json.Unmarshal(f.Value, &v); err != nil {
			t.Fatalf("%s: invalid json %s", f.FeatureKey, f.Value)
		}
		switch registry[f.FeatureKey] {
		case "boolean":
			if _, ok := v.(bool); !ok {
				t.Errorf("%s: want boolean, got %s", f.FeatureKey, f.Value)
			}
		case "integer":
			n, ok := v.(float64)
			if !ok || n != float64(int64(n)) || n < 0 {
				t.Errorf("%s: want non-negative integer, got %s", f.FeatureKey, f.Value)
			}
		default:
			t.Errorf("%s: unexpected registry type %q", f.FeatureKey, registry[f.FeatureKey])
		}
	}
}
