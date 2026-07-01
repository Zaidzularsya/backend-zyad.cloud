package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	billingmodel "zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/modules/billing/repository"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

type stubBillingGuardSubscriptionStore struct {
	usableErr error
	usable    billingmodel.Subscription
	latest    []billingmodel.Subscription
}

func (s stubBillingGuardSubscriptionStore) FindUsableByOrganization(
	context.Context,
	string,
) (billingmodel.Subscription, error) {
	return s.usable, s.usableErr
}

func (s stubBillingGuardSubscriptionStore) List(
	context.Context,
	repository.SubscriptionListFilter,
) ([]billingmodel.Subscription, int64, error) {
	return s.latest, int64(len(s.latest)), nil
}

type stubBillingGuardEntitlementEvaluator struct {
	entitlement organizationmodel.Entitlement
	err         error
}

func (s stubBillingGuardEntitlementEvaluator) RequireFeature(
	context.Context,
	string,
	string,
) (organizationmodel.Entitlement, error) {
	return s.entitlement, s.err
}

func TestBillingGuardBlocksSuspendedSubscription(t *testing.T) {
	service := NewBillingGuardService(stubBillingGuardSubscriptionStore{
		usableErr: pgx.ErrNoRows,
		latest: []billingmodel.Subscription{
			{Status: billingmodel.SubscriptionStatusSuspended},
		},
	}, stubBillingGuardEntitlementEvaluator{})

	_, err := service.RequireFeature(context.Background(), "organization-1", "landing.enabled")
	assertAppErrorCode(t, err, "SUBSCRIPTION_SUSPENDED")
}

func TestBillingGuardBlocksDisabledFeature(t *testing.T) {
	service := NewBillingGuardService(stubBillingGuardSubscriptionStore{
		usable: billingmodel.Subscription{Status: billingmodel.SubscriptionStatusActive},
	}, stubBillingGuardEntitlementEvaluator{
		err: coreerrors.New(
			"ORGANIZATION_FEATURE_NOT_ENTITLED",
			"organization feature is not enabled",
			http.StatusForbidden,
		),
	})

	_, err := service.RequireFeature(context.Background(), "organization-1", "landing.enabled")
	assertAppErrorCode(t, err, "FEATURE_NOT_ENABLED")
}

func TestBillingGuardBlocksQuotaExceeded(t *testing.T) {
	service := NewBillingGuardService(stubBillingGuardSubscriptionStore{
		usable: billingmodel.Subscription{Status: billingmodel.SubscriptionStatusActive},
	}, stubBillingGuardEntitlementEvaluator{
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

func TestBillingGuardAllowsFeatureAndQuota(t *testing.T) {
	service := NewBillingGuardService(stubBillingGuardSubscriptionStore{
		usable: billingmodel.Subscription{Status: billingmodel.SubscriptionStatusActive},
	}, stubBillingGuardEntitlementEvaluator{
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
