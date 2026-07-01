package service

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/modules/billing/repository"
)

type stubPlanEntitlementStore struct {
	received []repository.UpsertPlanEntitlementParams
}

func (s *stubPlanEntitlementStore) ListByPlanID(
	context.Context,
	string,
) ([]model.PlanEntitlement, error) {
	return nil, nil
}

func (s *stubPlanEntitlementStore) ReplaceByPlanID(
	_ context.Context,
	_ string,
	entitlements []repository.UpsertPlanEntitlementParams,
) ([]model.PlanEntitlement, error) {
	s.received = entitlements
	return []model.PlanEntitlement{}, nil
}

type stubFeatureStore struct {
	features map[string]model.Feature
}

func (s stubFeatureStore) FindByKey(_ context.Context, key string) (model.Feature, error) {
	return s.features[key], nil
}

func TestPlanEntitlementServiceReplaceRejectsInvalidValueType(t *testing.T) {
	store := &stubPlanEntitlementStore{}
	service := NewPlanEntitlementService(store, stubFeatureStore{
		features: map[string]model.Feature{
			"landing.enabled": {
				ID:        "feature-1",
				Key:       "landing.enabled",
				ValueType: model.FeatureValueTypeBoolean,
				IsActive:  true,
			},
		},
	})
	value := int64(1)

	_, err := service.ReplaceByPlanID(context.Background(), "plan-1", dto.ReplacePlanEntitlementsRequest{
		Entitlements: []dto.PlanEntitlementValueRequest{
			{
				FeatureKey: "landing.enabled",
				ValueInt:   &value,
			},
		},
	})
	if err == nil {
		t.Fatal("ReplaceByPlanID error = nil, want validation error")
	}
	if len(store.received) != 0 {
		t.Fatalf("ReplaceByPlanID stored %d entitlements, want 0", len(store.received))
	}
}

func TestPlanEntitlementServiceReplaceAcceptsMatchingValueType(t *testing.T) {
	store := &stubPlanEntitlementStore{}
	service := NewPlanEntitlementService(store, stubFeatureStore{
		features: map[string]model.Feature{
			"landing.enabled": {
				ID:        "feature-1",
				Key:       "landing.enabled",
				ValueType: model.FeatureValueTypeBoolean,
				IsActive:  true,
			},
		},
	})
	enabled := true

	_, err := service.ReplaceByPlanID(context.Background(), "plan-1", dto.ReplacePlanEntitlementsRequest{
		Entitlements: []dto.PlanEntitlementValueRequest{
			{
				FeatureKey: "landing.enabled",
				ValueBool:  &enabled,
			},
		},
	})
	if err != nil {
		t.Fatalf("ReplaceByPlanID error = %v", err)
	}
	if len(store.received) != 1 {
		t.Fatalf("ReplaceByPlanID stored %d entitlements, want 1", len(store.received))
	}
	if store.received[0].FeatureID != "feature-1" {
		t.Fatalf("FeatureID = %s, want feature-1", store.received[0].FeatureID)
	}
}
