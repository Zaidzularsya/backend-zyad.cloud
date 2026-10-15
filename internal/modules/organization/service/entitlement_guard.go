package service

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	organization "zyad.cloud/internal/modules/organization"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

type EntitlementEvaluator interface {
	RequireFeature(ctx context.Context, organizationID string, featureKey string) (organizationmodel.Entitlement, error)
}

// OrganizationTypeResolver resolves an organization's type so
// RequireFeature can bypass subscription/entitlement checks for platform-type
// organizations (Zyad itself never subscribes to a plan). Optional — nil
// preserves the pre-existing behavior (every organization goes through the
// full subscription check).
type OrganizationTypeResolver interface {
	FindByID(ctx context.Context, id string) (organizationmodel.Organization, error)
}

type QuotaInput struct {
	OrganizationID string
	FeatureKey     string
	LimitKey       string
	UsedValue      int64
	Delta          int64
}

type EntitlementGuard struct {
	entitlements  EntitlementEvaluator
	organizations OrganizationTypeResolver
	now           func() time.Time
}

type EntitlementGuardOption func(*EntitlementGuard)

// WithOrganizationTypeResolver enables the platform-organization bypass in
// RequireFeature. Without it, RequireFeature behaves exactly as before.
func WithOrganizationTypeResolver(store OrganizationTypeResolver) EntitlementGuardOption {
	return func(s *EntitlementGuard) {
		s.organizations = store
	}
}

func NewEntitlementGuard(
	entitlements EntitlementEvaluator,
	opts ...EntitlementGuardOption,
) *EntitlementGuard {
	service := &EntitlementGuard{
		entitlements: entitlements,
		now:          time.Now,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

func (s *EntitlementGuard) RequireFeature(
	ctx context.Context,
	organizationID string,
	featureKey string,
) (organizationmodel.Entitlement, error) {
	isPlatform, err := s.isPlatformOrganization(ctx, organizationID)
	if err != nil {
		return organizationmodel.Entitlement{}, err
	}
	if isPlatform {
		// Platform organizations never buy a product — treat them as
		// unconditionally entitled. The zero-value Entitlement has a nil
		// Limits map, which guardLimit() (used by RequireQuota) already
		// treats as "no limit", so this also covers quota guards called
		// directly from service layers (e.g. ContactService.requireCreateQuota),
		// not just the RequireEntitlement middleware.
		return organizationmodel.Entitlement{OrganizationID: organizationID, FeatureKey: featureKey}, nil
	}
	entitlement, err := s.requireEntitlement(ctx, organizationID, featureKey)
	if err != nil {
		return organizationmodel.Entitlement{}, err
	}
	return entitlement, nil
}

func (s *EntitlementGuard) isPlatformOrganization(ctx context.Context, organizationID string) (bool, error) {
	if s.organizations == nil {
		return false, nil
	}
	org, err := s.organizations.FindByID(ctx, strings.TrimSpace(organizationID))
	if err != nil {
		return false, err
	}
	return org.IsPlatform(), nil
}

func (s *EntitlementGuard) RequireQuota(ctx context.Context, input QuotaInput) (organizationmodel.Entitlement, error) {
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
		return organizationmodel.Entitlement{}, organization.QuotaExceededError()
	}
	return entitlement, nil
}

func (s *EntitlementGuard) RequireQuotaValue(
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

func (s *EntitlementGuard) requireEntitlement(
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
			return organizationmodel.Entitlement{}, organization.FeatureNotEnabledError()
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
