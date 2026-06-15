package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

const entitlementTestOrganizationID = "11111111-1111-1111-1111-111111111111"

type fakeEntitlementStore struct {
	entitlement    model.Entitlement
	findErr        error
	counter        model.UsageCounter
	usageErr       error
	incrementErr   error
	findAt         time.Time
	incrementKey   repository.UsageCounterKey
	incrementDelta int64
	incrementLimit *int64
}

func (f *fakeEntitlementStore) FindEffective(
	_ context.Context,
	_ string,
	_ string,
	at time.Time,
) (model.Entitlement, error) {
	f.findAt = at
	return f.entitlement, f.findErr
}

func (f *fakeEntitlementStore) GetUsage(
	_ context.Context,
	_ repository.UsageCounterKey,
) (model.UsageCounter, error) {
	return f.counter, f.usageErr
}

func (f *fakeEntitlementStore) IncrementUsage(
	_ context.Context,
	key repository.UsageCounterKey,
	delta int64,
	limit *int64,
	_ time.Time,
) (model.UsageCounter, error) {
	f.incrementKey = key
	f.incrementDelta = delta
	f.incrementLimit = limit
	return f.counter, f.incrementErr
}

func TestEntitlementServiceEvaluateFeatureFailsClosed(t *testing.T) {
	now := time.Date(2026, 6, 15, 2, 0, 0, 0, time.UTC)
	store := &fakeEntitlementStore{
		entitlement: model.Entitlement{
			OrganizationID: entitlementTestOrganizationID,
			FeatureKey:     "landing.enabled",
			Status:         model.EntitlementStatusActive,
			EffectiveFrom:  now.Add(-time.Hour),
			EffectiveUntil: timePointer(now),
		},
	}
	service := NewEntitlementService(store)
	service.now = func() time.Time { return now }

	evaluation, err := service.EvaluateFeature(
		context.Background(),
		entitlementTestOrganizationID,
		" Landing.Enabled ",
	)
	if err != nil {
		t.Fatalf("EvaluateFeature() error = %v", err)
	}
	if evaluation.Enabled {
		t.Fatal("expired entitlement must fail closed")
	}
}

func TestEntitlementServiceRequireFeatureMapsMissingEntitlement(t *testing.T) {
	service := NewEntitlementService(&fakeEntitlementStore{findErr: pgx.ErrNoRows})
	_, err := service.RequireFeature(
		context.Background(),
		entitlementTestOrganizationID,
		"landing.enabled",
	)
	assertAppErrorCode(t, err, "ORGANIZATION_FEATURE_NOT_ENTITLED")
}

func TestEntitlementServiceCheckUsageReturnsZeroForMissingCounter(t *testing.T) {
	now := time.Date(2026, 6, 15, 2, 0, 0, 0, time.UTC)
	store := activeEntitlementStore(now, map[string]any{"max_pages": float64(5)})
	store.usageErr = pgx.ErrNoRows
	service := NewEntitlementService(store)
	service.now = func() time.Time { return now }

	result, err := service.CheckUsage(context.Background(), usageTestInput())
	if err != nil {
		t.Fatalf("CheckUsage() error = %v", err)
	}
	if result.Counter.UsageValue != 0 || result.Limit == nil || *result.Limit != 5 ||
		result.Remaining == nil || *result.Remaining != 5 {
		t.Fatalf("CheckUsage() result = %#v", result)
	}
}

func TestEntitlementServiceConsumeUsageUsesAtomicHardLimit(t *testing.T) {
	now := time.Date(2026, 6, 15, 2, 0, 0, 0, time.UTC)
	store := activeEntitlementStore(now, map[string]any{"max_pages": int64(5)})
	store.counter = model.UsageCounter{UsageValue: 4}
	service := NewEntitlementService(store)
	service.now = func() time.Time { return now }

	result, err := service.ConsumeUsage(context.Background(), ConsumeUsageInput{
		UsageInput: usageTestInput(),
		Delta:      2,
	})
	if err != nil {
		t.Fatalf("ConsumeUsage() error = %v", err)
	}
	if store.incrementLimit == nil || *store.incrementLimit != 5 ||
		store.incrementDelta != 2 || result.Remaining == nil || *result.Remaining != 1 {
		t.Fatalf("ConsumeUsage() store/result = %#v / %#v", store, result)
	}
}

func TestEntitlementServiceConsumeUsageMapsLimitExceeded(t *testing.T) {
	now := time.Date(2026, 6, 15, 2, 0, 0, 0, time.UTC)
	store := activeEntitlementStore(now, map[string]any{"max_pages": float64(1)})
	store.incrementErr = repository.ErrUsageLimitExceeded
	service := NewEntitlementService(store)
	service.now = func() time.Time { return now }

	_, err := service.ConsumeUsage(context.Background(), ConsumeUsageInput{
		UsageInput: usageTestInput(),
		Delta:      1,
	})
	assertAppErrorCode(t, err, "ORGANIZATION_USAGE_LIMIT_EXCEEDED")
}

func TestEntitlementServiceRejectsInvalidLimit(t *testing.T) {
	now := time.Date(2026, 6, 15, 2, 0, 0, 0, time.UTC)
	store := activeEntitlementStore(now, map[string]any{"max_pages": 1.5})
	service := NewEntitlementService(store)
	service.now = func() time.Time { return now }

	_, err := service.CheckUsage(context.Background(), usageTestInput())
	assertAppErrorCode(t, err, "ENTITLEMENT_LIMIT_INVALID")
}

func activeEntitlementStore(now time.Time, limits map[string]any) *fakeEntitlementStore {
	return &fakeEntitlementStore{
		entitlement: model.Entitlement{
			OrganizationID: entitlementTestOrganizationID,
			FeatureKey:     "landing.max_pages",
			Status:         model.EntitlementStatusActive,
			Limits:         limits,
			EffectiveFrom:  now.Add(-time.Hour),
		},
	}
}

func usageTestInput() UsageInput {
	return UsageInput{
		OrganizationID: entitlementTestOrganizationID,
		FeatureKey:     "landing.max_pages",
		MetricKey:      "page_count",
		LimitKey:       "max_pages",
		PeriodStart:    time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:      time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func assertAppErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("error = %v, want app error %s", err, code)
	}
}
