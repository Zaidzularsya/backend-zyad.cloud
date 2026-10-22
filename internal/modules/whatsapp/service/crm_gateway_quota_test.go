package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/organization"
)

type stubLeadQuota struct {
	err  error
	keys []string
}

func (s *stubLeadQuota) RequireRoom(_ context.Context, _ string, key string) error {
	s.keys = append(s.keys, key)
	return s.err
}

func TestCRMGatewayCreateLeadSkippedWhenQuotaUnavailable(t *testing.T) {
	for name, quotaErr := range map[string]error{
		"quota exceeded":      organization.QuotaExceededError(),
		"feature not enabled": organization.FeatureNotEnabledError(),
	} {
		quota := &stubLeadQuota{err: quotaErr}
		// nil repositories: the quota check must stop before any repo call.
		gateway := NewCRMGateway(nil, nil, nil, nil, WithLeadQuota(quota))

		_, err := gateway.CreateLead(context.Background(), coretenant.Scope{}, CreateLeadInput{ContactName: "x"})
		if !errors.Is(err, ErrAutoLeadSkipped) {
			t.Errorf("%s: err = %v, want ErrAutoLeadSkipped", name, err)
		}
		if len(quota.keys) != 1 || quota.keys[0] != crmdomain.FeatureCRMMaxLeads {
			t.Errorf("%s: keys = %v", name, quota.keys)
		}
	}
}

func TestCRMGatewayCreateLeadPropagatesOtherQuotaErrors(t *testing.T) {
	boom := errors.New("db down")
	gateway := NewCRMGateway(nil, nil, nil, nil, WithLeadQuota(&stubLeadQuota{err: boom}))

	_, err := gateway.CreateLead(context.Background(), coretenant.Scope{}, CreateLeadInput{ContactName: "x"})
	if !errors.Is(err, boom) || errors.Is(err, ErrAutoLeadSkipped) {
		t.Fatalf("err = %v, want the original error", err)
	}
}
