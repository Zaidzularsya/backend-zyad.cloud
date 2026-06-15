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
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

type EntitlementStore interface {
	FindEffective(context.Context, string, string, time.Time) (model.Entitlement, error)
	GetUsage(context.Context, repository.UsageCounterKey) (model.UsageCounter, error)
	IncrementUsage(
		context.Context,
		repository.UsageCounterKey,
		int64,
		*int64,
		time.Time,
	) (model.UsageCounter, error)
}

type FeatureEvaluation struct {
	Enabled     bool
	Entitlement model.Entitlement
}

type UsageEvaluation struct {
	Counter   model.UsageCounter
	Limit     *int64
	Remaining *int64
}

type UsageInput struct {
	OrganizationID string
	FeatureKey     string
	MetricKey      string
	LimitKey       string
	PeriodStart    time.Time
	PeriodEnd      time.Time
}

type ConsumeUsageInput struct {
	UsageInput
	Delta int64
}

type EntitlementService struct {
	store EntitlementStore
	now   func() time.Time
}

func NewEntitlementService(store EntitlementStore) *EntitlementService {
	return &EntitlementService{store: store, now: time.Now}
}

func (s *EntitlementService) EvaluateFeature(
	ctx context.Context,
	organizationID string,
	featureKey string,
) (FeatureEvaluation, error) {
	organizationID, featureKey, err := normalizeFeatureInput(organizationID, featureKey)
	if err != nil {
		return FeatureEvaluation{}, err
	}
	if s.store == nil {
		return FeatureEvaluation{}, entitlementStoreRequiredError()
	}

	now := s.now().UTC()
	entitlement, err := s.store.FindEffective(ctx, organizationID, featureKey, now)
	if errors.Is(err, pgx.ErrNoRows) {
		return FeatureEvaluation{}, nil
	}
	if err != nil {
		return FeatureEvaluation{}, coreerrors.Wrap(
			"ENTITLEMENT_EVALUATION_FAILED",
			"failed to evaluate organization entitlement",
			http.StatusInternalServerError,
			err,
		)
	}
	if entitlement.OrganizationID != organizationID ||
		canonicalKey(entitlement.FeatureKey) != featureKey ||
		!entitlement.IsEffective(now) {
		return FeatureEvaluation{}, nil
	}
	if entitlement.Limits == nil {
		entitlement.Limits = map[string]any{}
	}
	return FeatureEvaluation{Enabled: true, Entitlement: entitlement}, nil
}

func (s *EntitlementService) RequireFeature(
	ctx context.Context,
	organizationID string,
	featureKey string,
) (model.Entitlement, error) {
	evaluation, err := s.EvaluateFeature(ctx, organizationID, featureKey)
	if err != nil {
		return model.Entitlement{}, err
	}
	if !evaluation.Enabled {
		return model.Entitlement{}, coreerrors.New(
			"ORGANIZATION_FEATURE_NOT_ENTITLED",
			"organization feature is not enabled",
			http.StatusForbidden,
		)
	}
	return evaluation.Entitlement, nil
}

func (s *EntitlementService) CheckUsage(
	ctx context.Context,
	input UsageInput,
) (UsageEvaluation, error) {
	key, limit, err := s.prepareUsage(ctx, input)
	if err != nil {
		return UsageEvaluation{}, err
	}
	counter, err := s.store.GetUsage(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		counter = emptyUsageCounter(key)
	} else if err != nil {
		return UsageEvaluation{}, coreerrors.Wrap(
			"ENTITLEMENT_USAGE_READ_FAILED",
			"failed to read organization usage",
			http.StatusInternalServerError,
			err,
		)
	}
	return newUsageEvaluation(counter, limit), nil
}

func (s *EntitlementService) ConsumeUsage(
	ctx context.Context,
	input ConsumeUsageInput,
) (UsageEvaluation, error) {
	if input.Delta <= 0 {
		return UsageEvaluation{}, validationError("usage delta must be greater than zero")
	}
	key, limit, err := s.prepareUsage(ctx, input.UsageInput)
	if err != nil {
		return UsageEvaluation{}, err
	}
	counter, err := s.store.IncrementUsage(ctx, key, input.Delta, limit, s.now().UTC())
	if errors.Is(err, repository.ErrUsageLimitExceeded) {
		return UsageEvaluation{}, coreerrors.New(
			"ORGANIZATION_USAGE_LIMIT_EXCEEDED",
			"organization usage limit has been exceeded",
			http.StatusConflict,
		)
	}
	if err != nil {
		return UsageEvaluation{}, coreerrors.Wrap(
			"ENTITLEMENT_USAGE_INCREMENT_FAILED",
			"failed to increment organization usage",
			http.StatusInternalServerError,
			err,
		)
	}
	return newUsageEvaluation(counter, limit), nil
}

func (s *EntitlementService) prepareUsage(
	ctx context.Context,
	input UsageInput,
) (repository.UsageCounterKey, *int64, error) {
	organizationID, featureKey, err := normalizeFeatureInput(
		input.OrganizationID,
		input.FeatureKey,
	)
	if err != nil {
		return repository.UsageCounterKey{}, nil, err
	}
	metricKey := canonicalKey(input.MetricKey)
	limitKey := strings.TrimSpace(input.LimitKey)
	if metricKey == "" || limitKey == "" {
		return repository.UsageCounterKey{}, nil,
			validationError("metric_key and limit_key are required")
	}
	periodStart := input.PeriodStart.UTC()
	periodEnd := input.PeriodEnd.UTC()
	if periodStart.IsZero() || !periodEnd.After(periodStart) {
		return repository.UsageCounterKey{}, nil,
			validationError("usage period is invalid")
	}

	entitlement, err := s.RequireFeature(ctx, organizationID, featureKey)
	if err != nil {
		return repository.UsageCounterKey{}, nil, err
	}
	limit, err := entitlementLimit(entitlement.Limits, limitKey)
	if err != nil {
		return repository.UsageCounterKey{}, nil, err
	}
	return repository.UsageCounterKey{
		OrganizationID: organizationID,
		FeatureKey:     featureKey,
		MetricKey:      metricKey,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
	}, limit, nil
}

func normalizeFeatureInput(organizationID, featureKey string) (string, string, error) {
	organizationID = strings.TrimSpace(organizationID)
	featureKey = canonicalKey(featureKey)
	if !validUUID(organizationID) {
		return "", "", validationError("organization_id must be a valid UUID")
	}
	if featureKey == "" {
		return "", "", validationError("feature_key is required")
	}
	return organizationID, featureKey, nil
}

func entitlementLimit(limits map[string]any, key string) (*int64, error) {
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
			return nil, invalidEntitlementLimitError(key)
		}
		limit = int64(typed)
	default:
		return nil, invalidEntitlementLimitError(key)
	}
	if limit < 0 {
		return nil, invalidEntitlementLimitError(key)
	}
	return &limit, nil
}

func newUsageEvaluation(counter model.UsageCounter, limit *int64) UsageEvaluation {
	var remaining *int64
	if limit != nil {
		value := *limit - counter.UsageValue
		if value < 0 {
			value = 0
		}
		remaining = &value
	}
	return UsageEvaluation{Counter: counter, Limit: limit, Remaining: remaining}
}

func emptyUsageCounter(key repository.UsageCounterKey) model.UsageCounter {
	return model.UsageCounter{
		OrganizationID: key.OrganizationID,
		FeatureKey:     key.FeatureKey,
		MetricKey:      key.MetricKey,
		PeriodStart:    key.PeriodStart,
		PeriodEnd:      key.PeriodEnd,
	}
}

func canonicalKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func invalidEntitlementLimitError(key string) error {
	return coreerrors.New(
		"ENTITLEMENT_LIMIT_INVALID",
		"entitlement limit "+key+" must be a non-negative integer",
		http.StatusUnprocessableEntity,
	)
}

func entitlementStoreRequiredError() error {
	return coreerrors.New(
		"ENTITLEMENT_STORE_REQUIRED",
		"entitlement store is required",
		http.StatusInternalServerError,
	)
}
