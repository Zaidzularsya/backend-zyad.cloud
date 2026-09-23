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

// SubscriptionGuardOrganizationStore resolves an organization's type so
// RequireFeature can bypass subscription/entitlement checks for platform-type
// organizations (Zyad itself never subscribes to a plan). Optional — nil
// preserves the pre-existing behavior (every organization goes through the
// full subscription check).
type SubscriptionGuardOrganizationStore interface {
	FindByID(ctx context.Context, id string) (organizationmodel.Organization, error)
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
	organizations SubscriptionGuardOrganizationStore
	now           func() time.Time
}

type SubscriptionGuardOption func(*SubscriptionGuardService)

// WithOrganizationTypeResolver enables the platform-organization bypass in
// RequireFeature. Without it, RequireFeature behaves exactly as before.
func WithOrganizationTypeResolver(store SubscriptionGuardOrganizationStore) SubscriptionGuardOption {
	return func(s *SubscriptionGuardService) {
		s.organizations = store
	}
}

func NewSubscriptionGuardService(
	subscriptions SubscriptionGuardSubscriptionStore,
	entitlements SubscriptionGuardEntitlementEvaluator,
	opts ...SubscriptionGuardOption,
) *SubscriptionGuardService {
	service := &SubscriptionGuardService{
		subscriptions: subscriptions,
		entitlements:  entitlements,
		now:           time.Now,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

func (s *SubscriptionGuardService) RequireFeature(
	ctx context.Context,
	organizationID string,
	featureKey string,
) (organizationmodel.Entitlement, error) {
	isPlatform, err := s.isPlatformOrganization(ctx, organizationID)
	if err != nil {
		return organizationmodel.Entitlement{}, err
	}
	if isPlatform {
		// Platform organizations never subscribe to a plan — treat them as
		// unconditionally entitled. The zero-value Entitlement has a nil
		// Limits map, which guardLimit() (used by RequireQuota) already
		// treats as "no limit", so this also covers quota guards called
		// directly from service layers (e.g. ContactService.requireCreateQuota),
		// not just the RequireEntitlement middleware.
		return organizationmodel.Entitlement{OrganizationID: organizationID, FeatureKey: featureKey}, nil
	}
	if err := s.requireUsableSubscription(ctx, organizationID); err != nil {
		return organizationmodel.Entitlement{}, err
	}
	entitlement, err := s.requireEntitlement(ctx, organizationID, featureKey)
	if err != nil {
		return organizationmodel.Entitlement{}, err
	}
	return entitlement, nil
}

func (s *SubscriptionGuardService) isPlatformOrganization(ctx context.Context, organizationID string) (bool, error) {
	if s.organizations == nil {
		return false, nil
	}
	org, err := s.organizations.FindByID(ctx, strings.TrimSpace(organizationID))
	if err != nil {
		return false, err
	}
	return org.IsPlatform(), nil
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
