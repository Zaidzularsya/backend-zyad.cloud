package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	subscriptionmodel "zyad.cloud/internal/modules/subscription/model"
	"zyad.cloud/internal/modules/subscription/repository"
)

type stubSubscriptionGuardOrganizationStore struct {
	organization organizationmodel.Organization
	err          error
}

func (s stubSubscriptionGuardOrganizationStore) FindByID(
	context.Context,
	string,
) (organizationmodel.Organization, error) {
	return s.organization, s.err
}

type stubSubscriptionGuardSubscriptionStore struct {
	usableErr error
	usable    subscriptionmodel.Subscription
	latest    []subscriptionmodel.Subscription
}

func (s stubSubscriptionGuardSubscriptionStore) FindUsableByOrganization(
	context.Context,
	string,
) (subscriptionmodel.Subscription, error) {
	return s.usable, s.usableErr
}

func (s stubSubscriptionGuardSubscriptionStore) List(
	context.Context,
	repository.SubscriptionListFilter,
) ([]subscriptionmodel.Subscription, int64, error) {
	return s.latest, int64(len(s.latest)), nil
}

type stubSubscriptionGuardEntitlementEvaluator struct {
	entitlement organizationmodel.Entitlement
	err         error
}

func (s stubSubscriptionGuardEntitlementEvaluator) RequireFeature(
	context.Context,
	string,
	string,
) (organizationmodel.Entitlement, error) {
	return s.entitlement, s.err
}

func TestSubscriptionGuardBlocksSuspendedSubscription(t *testing.T) {
	service := NewSubscriptionGuardService(stubSubscriptionGuardSubscriptionStore{
		usableErr: pgx.ErrNoRows,
		latest: []subscriptionmodel.Subscription{
			{Status: subscriptionmodel.SubscriptionStatusSuspended},
		},
	}, stubSubscriptionGuardEntitlementEvaluator{})

	_, err := service.RequireFeature(context.Background(), "organization-1", "landing.enabled")
	assertAppErrorCode(t, err, "SUBSCRIPTION_SUSPENDED")
}

func TestSubscriptionGuardBlocksDisabledFeature(t *testing.T) {
	service := NewSubscriptionGuardService(stubSubscriptionGuardSubscriptionStore{
		usable: subscriptionmodel.Subscription{Status: subscriptionmodel.SubscriptionStatusActive},
	}, stubSubscriptionGuardEntitlementEvaluator{
		err: coreerrors.New(
			"ORGANIZATION_FEATURE_NOT_ENTITLED",
			"organization feature is not enabled",
			http.StatusForbidden,
		),
	})

	_, err := service.RequireFeature(context.Background(), "organization-1", "landing.enabled")
	assertAppErrorCode(t, err, "FEATURE_NOT_ENABLED")
}

func TestSubscriptionGuardBlocksQuotaExceeded(t *testing.T) {
	service := NewSubscriptionGuardService(stubSubscriptionGuardSubscriptionStore{
		usable: subscriptionmodel.Subscription{Status: subscriptionmodel.SubscriptionStatusActive},
	}, stubSubscriptionGuardEntitlementEvaluator{
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

func TestSubscriptionGuardAllowsFeatureAndQuota(t *testing.T) {
	service := NewSubscriptionGuardService(stubSubscriptionGuardSubscriptionStore{
		usable: subscriptionmodel.Subscription{Status: subscriptionmodel.SubscriptionStatusActive},
	}, stubSubscriptionGuardEntitlementEvaluator{
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

func TestSubscriptionGuardBypassesPlatformOrganization(t *testing.T) {
	// The subscription store is set up to fail (no rows, no latest
	// subscription) so that if the bypass didn't actually skip
	// requireUsableSubscription, this test would fail with
	// SUBSCRIPTION_NOT_FOUND instead of succeeding.
	service := NewSubscriptionGuardService(
		stubSubscriptionGuardSubscriptionStore{usableErr: pgx.ErrNoRows},
		stubSubscriptionGuardEntitlementEvaluator{
			err: coreerrors.New("ORGANIZATION_FEATURE_NOT_ENTITLED", "not entitled", http.StatusForbidden),
		},
		WithOrganizationTypeResolver(stubSubscriptionGuardOrganizationStore{
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

func TestSubscriptionGuardStillEnforcesCustomerOrganization(t *testing.T) {
	service := NewSubscriptionGuardService(
		stubSubscriptionGuardSubscriptionStore{usableErr: pgx.ErrNoRows},
		stubSubscriptionGuardEntitlementEvaluator{},
		WithOrganizationTypeResolver(stubSubscriptionGuardOrganizationStore{
			organization: organizationmodel.Organization{Type: coretenant.OrganizationTypeCustomer},
		}),
	)

	_, err := service.RequireFeature(context.Background(), "tenant-org", "crm.enabled")
	assertAppErrorCode(t, err, "SUBSCRIPTION_NOT_FOUND")
}

func assertAppErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", code)
	}
	appErr, ok := err.(*coreerrors.AppError)
	if !ok {
		t.Fatalf("error type = %T, want *AppError", err)
	}
	if appErr.Code != code {
		t.Fatalf("error code = %s, want %s", appErr.Code, code)
	}
}
