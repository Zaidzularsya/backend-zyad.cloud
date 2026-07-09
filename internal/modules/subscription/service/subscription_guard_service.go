package service

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	subscription "zyad.cloud/internal/modules/subscription"
	subscriptionmodel "zyad.cloud/internal/modules/subscription/model"
	"zyad.cloud/internal/modules/subscription/repository"
)

type SubscriptionGuardSubscriptionStore interface {
	FindUsableByOrganization(ctx context.Context, organizationID string) (subscriptionmodel.Subscription, error)
	List(ctx context.Context, filter repository.SubscriptionListFilter) ([]subscriptionmodel.Subscription, int64, error)
}

type SubscriptionGuardEntitlementEvaluator interface {
	RequireFeature(ctx context.Context, organizationID string, featureKey string) (organizationmodel.Entitlement, error)
}

type QuotaInput struct {
	OrganizationID string
	FeatureKey     string
	LimitKey       string
	UsedValue      int64
	Delta          int64
}

type SubscriptionGuardService struct {
	subscriptions SubscriptionGuardSubscriptionStore
	entitlements  SubscriptionGuardEntitlementEvaluator
	now           func() time.Time
}

func NewSubscriptionGuardService(
	subscriptions SubscriptionGuardSubscriptionStore,
	entitlements SubscriptionGuardEntitlementEvaluator,
) *SubscriptionGuardService {
	return &SubscriptionGuardService{
		subscriptions: subscriptions,
		entitlements:  entitlements,
		now:           time.Now,
	}
}

func (s *SubscriptionGuardService) RequireFeature(
	ctx context.Context,
	organizationID string,
	featureKey string,
) (organizationmodel.Entitlement, error) {
	if err := s.requireUsableSubscription(ctx, organizationID); err != nil {
		return organizationmodel.Entitlement{}, err
	}
	entitlement, err := s.requireEntitlement(ctx, organizationID, featureKey)
	if err != nil {
		return organizationmodel.Entitlement{}, err
	}
	return entitlement, nil
}

func (s *SubscriptionGuardService) RequireQuota(ctx context.Context, input QuotaInput) (organizationmodel.Entitlement, error) {
	if input.Delta <= 0 {
		input.Delta = 1
	}
	entitlement, err := s.RequireFeature(ctx, input.OrganizationID, input.FeatureKey)
	if err != nil {
		return organizationmodel.Entitlement{}, err
	}
	limit, err := guardLimit(entitlement.Limits, input.LimitKey)
	if err != nil {
		return organizationmodel.Entitlement{}, err
	}
	if limit != nil && input.UsedValue+input.Delta > *limit {
		return organizationmodel.Entitlement{}, subscription.QuotaExceededError()
	}
	return entitlement, nil
}

func (s *SubscriptionGuardService) RequireQuotaValue(
	ctx context.Context,
	organizationID string,
	featureKey string,
	limitKey string,
	usedValue int64,
	delta int64,
) error {
	_, err := s.RequireQuota(ctx, QuotaInput{
		OrganizationID: organizationID,
		FeatureKey:     featureKey,
		LimitKey:       limitKey,
		UsedValue:      usedValue,
		Delta:          delta,
	})
	return err
}

func (s *SubscriptionGuardService) requireUsableSubscription(ctx context.Context, organizationID string) error {
	if s == nil || s.subscriptions == nil {
		return coreerrors.New(
			"BILLING_SUBSCRIPTION_STORE_REQUIRED",
			"billing subscription store is required",
			http.StatusInternalServerError,
		)
	}
	organizationID = strings.TrimSpace(organizationID)
	subscriptionRecord, err := s.subscriptions.FindUsableByOrganization(ctx, organizationID)
	if err == nil && subscriptionRecord.IsUsable() {
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	latest, latestErr := s.latestSubscription(ctx, organizationID)
	if latestErr != nil {
		return latestErr
	}
	switch latest.Status {
	case subscriptionmodel.SubscriptionStatusSuspended:
		return subscription.SubscriptionSuspendedError()
	case subscriptionmodel.SubscriptionStatusCanceled,
		subscriptionmodel.SubscriptionStatusExpired:
		return subscription.SubscriptionInactiveError()
	default:
		return subscription.SubscriptionNotFoundError()
	}
}

func (s *SubscriptionGuardService) latestSubscription(ctx context.Context, organizationID string) (subscriptionmodel.Subscription, error) {
	subscriptions, _, err := s.subscriptions.List(ctx, repository.SubscriptionListFilter{
		OrganizationID: strings.TrimSpace(organizationID),
		Limit:          1,
	})
	if err != nil {
		return subscriptionmodel.Subscription{}, err
	}
	if len(subscriptions) == 0 {
		return subscriptionmodel.Subscription{}, subscription.SubscriptionNotFoundError()
	}
	return subscriptions[0], nil
}

func (s *SubscriptionGuardService) requireEntitlement(
	ctx context.Context,
	organizationID string,
	featureKey string,
) (organizationmodel.Entitlement, error) {
	if s == nil || s.entitlements == nil {
		return organizationmodel.Entitlement{}, coreerrors.New(
			"BILLING_ENTITLEMENT_EVALUATOR_REQUIRED",
			"billing entitlement evaluator is required",
			http.StatusInternalServerError,
		)
	}
	entitlement, err := s.entitlements.RequireFeature(ctx, strings.TrimSpace(organizationID), strings.TrimSpace(featureKey))
	if err != nil {
		var appErr *coreerrors.AppError
		if errors.As(err, &appErr) && appErr.Code == "ORGANIZATION_FEATURE_NOT_ENTITLED" {
			return organizationmodel.Entitlement{}, subscription.FeatureNotEnabledError()
		}
		return organizationmodel.Entitlement{}, err
	}
	return entitlement, nil
}

func guardLimit(limits map[string]any, key string) (*int64, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, validationError("limit key is required")
	}
	if limits == nil {
		return nil, nil
	}
	value, exists := limits[key]
	if !exists || value == nil {
		return nil, nil
	}
	var limit int64
	switch typed := value.(type) {
	case int:
		limit = int64(typed)
	case int32:
		limit = int64(typed)
	case int64:
		limit = typed
	case float64:
		if math.Trunc(typed) != typed || typed > math.MaxInt64 {
			return nil, validationError("entitlement limit is invalid")
		}
		limit = int64(typed)
	default:
		return nil, validationError("entitlement limit is invalid")
	}
	if limit < 0 {
		return nil, validationError("entitlement limit is invalid")
	}
	return &limit, nil
}
