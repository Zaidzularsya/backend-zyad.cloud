package repository

import (
	"strings"
	"testing"

	"zyad.cloud/internal/modules/organization/model"
)

func TestEntitlementWhereCanonicalizesFeatureKey(t *testing.T) {
	where, args := entitlementWhere(EntitlementListFilter{
		FeatureKey: " Landing.Enabled ",
		Source:     model.EntitlementSourcePlan,
	})
	if !strings.Contains(where, "feature_key = $1") ||
		!strings.Contains(where, "source = $2") {
		t.Fatalf("entitlementWhere() = %q", where)
	}
	if len(args) != 2 || args[0] != "landing.enabled" ||
		args[1] != string(model.EntitlementSourcePlan) {
		t.Fatalf("entitlementWhere() args = %v", args)
	}
}

func TestCanonicalMetricKey(t *testing.T) {
	if got := canonicalMetricKey(" Page_Count "); got != "page_count" {
		t.Fatalf("canonicalMetricKey() = %q", got)
	}
}
