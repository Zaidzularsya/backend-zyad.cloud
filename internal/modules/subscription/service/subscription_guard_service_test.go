package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	subscriptionmodel "zyad.cloud/internal/modules/subscription/model"
	"zyad.cloud/internal/modules/subscription/repository"
)

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
