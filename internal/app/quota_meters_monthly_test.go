package app

import (
	"context"
	"testing"

	crmdomain "zyad.cloud/internal/modules/crm/domain"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
)

func TestMonthlyQuotaMetersAreRegistered(t *testing.T) {
	reg := organizationservice.NewQuotaMeterRegistry()
	registerMonthlyQuotaMeters(reg, nil)
	for _, key := range []string{
		crmdomain.FeatureCRMMaxQuotationsPerMonth,
		crmdomain.FeatureCRMMaxSalesOrdersPerMonth,
		receivabledomain.FeatureReceivableMaxInvoicesPerMonth,
	} {
		if _, ok := reg.Get(key); !ok {
			t.Errorf("meter %s not registered", key)
		}
	}
}

func TestMonthlyMeterRequiresPeriodStart(t *testing.T) {
	// A nil period start means the feature registry has reset_strategy=never for a
	// *PerMonth key; fail loudly instead of silently counting all rows ever created.
	if _, err := (scopedMonthlyCountMeter{}).Used(context.Background(), "org", nil); err == nil {
		t.Fatal("Used(nil periodStart) error = nil, want error")
	}
}
