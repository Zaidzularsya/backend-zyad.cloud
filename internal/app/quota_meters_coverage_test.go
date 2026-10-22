package app

import (
	"bytes"
	"encoding/json"
	"testing"
)

// noUsageMeterReason lists integer Free-plan keys that deliberately have no usage
// meter, with the reason. Anything NOT listed here and holding an integer value
// is a usage quota and MUST have a registered meter, otherwise the entitlement
// summary can never report used/at_limit for it and the "kuota penuh" banner
// would silently never show.
var noUsageMeterReason = map[string]string{
	"landing.max_sections_per_page": "per-item cap checked against one page at write time, not a tenant-wide usage counter",
	"media.max_file_size_mb":        "per-item cap on a single upload, not a tenant-wide usage counter",
	"media.max_storage_mb":          "catalog display only; the enforced storage key is storage.max_bytes",
}

func TestEveryUsageQuotaKeyInFreePlanHasMeter(t *testing.T) {
	// Registration never touches the DB, so a nil pool is fine here.
	reg := newQuotaMeterRegistry(nil)

	for _, feature := range FreePlanFeatures() {
		if !isJSONInteger(feature.Value) {
			continue // boolean flags are not quotas
		}
		if reason, excluded := noUsageMeterReason[feature.FeatureKey]; excluded {
			if reason == "" {
				t.Errorf("exclusion for %s needs a reason", feature.FeatureKey)
			}
			continue
		}
		if _, ok := reg.Get(feature.FeatureKey); !ok {
			t.Errorf("quota key %s has no registered QuotaMeter (add one in quota_meters.go or list it in noUsageMeterReason with a reason)", feature.FeatureKey)
		}
	}
}

func isJSONInteger(raw json.RawMessage) bool {
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&n); err != nil {
		return false
	}
	_, err := n.Int64()
	return err == nil
}
