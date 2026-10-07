package service

import (
	"context"
	"net/http"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

type stubOrganizationTypeResolver struct {
	organization organizationmodel.Organization
	err          error
}

func (s stubOrganizationTypeResolver) FindByID(
	context.Context,
	string,
) (organizationmodel.Organization, error) {
	return s.organization, s.err
}

type stubEntitlementEvaluator struct {
	entitlement organizationmodel.Entitlement
	err         error
}

func (s stubEntitlementEvaluator) RequireFeature(
	context.Context,
	string,
	string,
) (organizationmodel.Entitlement, error) {
	return s.entitlement, s.err
}

func TestEntitlementGuardBlocksDisabledFeature(t *testing.T) {
	service := NewEntitlementGuard(stubEntitlementEvaluator{
		err: coreerrors.New(
			"ORGANIZATION_FEATURE_NOT_ENTITLED",
			"organization feature is not enabled",
			http.StatusForbidden,
		),
	})

	_, err := service.RequireFeature(context.Background(), "organization-1", "landing.enabled")
	assertAppErrorCode(t, err, "FEATURE_NOT_ENABLED")
}

func TestEntitlementGuardBlocksQuotaExceeded(t *testing.T) {
	service := NewEntitlementGuard(stubEntitlementEvaluator{
		entitlement: organizationmodel.Entitlement{
			FeatureKey: "landing.max_pages",
			Limits:     map[string]any{"limit": int64(3)},
		},
	})

	_, err := service.RequireQuota(context.Background(), QuotaInput{
		OrganizationID: "organization-1",
		FeatureKey:     "landing.max_pages",
		LimitKey:       "limit",
		UsedValue:      3,
		Delta:          1,
	})
	assertAppErrorCode(t, err, "QUOTA_EXCEEDED")
}

func TestEntitlementGuardAllowsFeatureAndQuota(t *testing.T) {
	service := NewEntitlementGuard(stubEntitlementEvaluator{
		entitlement: organizationmodel.Entitlement{
			FeatureKey: "landing.max_pages",
			Limits:     map[string]any{"limit": float64(5)},
		},
	})

	entitlement, err := service.RequireQuota(context.Background(), QuotaInput{
		OrganizationID: "organization-1",
		FeatureKey:     "landing.max_pages",
		LimitKey:       "limit",
		UsedValue:      4,
		Delta:          1,
	})
	if err != nil {
		t.Fatalf("RequireQuota error = %v", err)
	}
	if entitlement.FeatureKey != "landing.max_pages" {
		t.Fatalf("FeatureKey = %s, want landing.max_pages", entitlement.FeatureKey)
	}
}

func TestEntitlementGuardBypassesPlatformOrganization(t *testing.T) {
	// The evaluator denies every feature, so the call can only succeed
	// through the platform bypass.
	service := NewEntitlementGuard(
		stubEntitlementEvaluator{
			err: coreerrors.New("ORGANIZATION_FEATURE_NOT_ENTITLED", "not entitled", http.StatusForbidden),
		},
		WithOrganizationTypeResolver(stubOrganizationTypeResolver{
			organization: organizationmodel.Organization{Type: coretenant.OrganizationTypePlatform},
		}),
	)

	entitlement, err := service.RequireFeature(context.Background(), "platform-org", "crm.enabled")
	if err != nil {
		t.Fatalf("RequireFeature() error = %v, want nil (platform bypass)", err)
	}
	if entitlement.FeatureKey != "crm.enabled" {
		t.Fatalf("FeatureKey = %s, want crm.enabled", entitlement.FeatureKey)
	}
}

func TestEntitlementGuardStillEnforcesCustomerOrganization(t *testing.T) {
	service := NewEntitlementGuard(
		stubEntitlementEvaluator{
			err: coreerrors.New("ORGANIZATION_FEATURE_NOT_ENTITLED", "not entitled", http.StatusForbidden),
		},
		WithOrganizationTypeResolver(stubOrganizationTypeResolver{
			organization: organizationmodel.Organization{Type: coretenant.OrganizationTypeCustomer},
		}),
	)

	_, err := service.RequireFeature(context.Background(), "tenant-org", "crm.enabled")
	assertAppErrorCode(t, err, "FEATURE_NOT_ENABLED")
}

// Workspace tanpa baris customer_subscriptions (paket gratis dari produk FREE, atau contract) harus lolos
// selama entitlement-nya aktif: guard tidak lagi bergantung pada subscription.
func TestEntitlementGuardAllowsWorkspaceWithoutSubscription(t *testing.T) {
	service := NewEntitlementGuard(
		stubEntitlementEvaluator{
			entitlement: organizationmodel.Entitlement{
				FeatureKey: "crm.enabled",
				Source:     organizationmodel.EntitlementSourceDefault,
			},
		},
		WithOrganizationTypeResolver(stubOrganizationTypeResolver{
			organization: organizationmodel.Organization{Type: coretenant.OrganizationTypeCustomer},
		}),
	)

	entitlement, err := service.RequireFeature(context.Background(), "tenant-org", "crm.enabled")
	if err != nil || entitlement.Source != organizationmodel.EntitlementSourceDefault {
		t.Fatalf("RequireFeature() = %+v, %v; want default entitlement and no error", entitlement, err)
	}
}
