package domain

import (
	"regexp"
	"testing"

	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
)

func TestQuotaFeatureKeysMatchRegistryFormat(t *testing.T) {
	keys := []string{
		FeatureCRMMaxLeads,
		FeatureCRMMaxCompanies,
		FeatureCRMMaxDeals,
		FeatureCRMMaxPipelines,
		FeatureCRMMaxQuotationsPerMonth,
		FeatureCRMMaxSalesOrdersPerMonth,
		receivabledomain.FeatureReceivableMaxInvoicesPerMonth,
	}
	re := regexp.MustCompile(`^[a-z][a-z0-9_]*([.][a-z][a-z0-9_]*)+$`)
	seen := map[string]bool{}
	for _, k := range keys {
		if !re.MatchString(k) {
			t.Errorf("key %q does not match registry format", k)
		}
		if seen[k] {
			t.Errorf("duplicate key %q", k)
		}
		seen[k] = true
	}
}
