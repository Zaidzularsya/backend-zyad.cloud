package service

import (
	"context"
	"strings"

	"zyad.cloud/internal/modules/product/dto"
	"zyad.cloud/internal/modules/product/model"
	"zyad.cloud/internal/modules/product/repository"
)

type PlanEntitlementStore interface {
	ListByPlanID(ctx context.Context, planID string) ([]model.PlanEntitlement, error)
	ReplaceByPlanID(
		ctx context.Context,
		planID string,
		entitlements []repository.UpsertPlanEntitlementParams,
	) ([]model.PlanEntitlement, error)
}

type PlanEntitlementFeatureStore interface {
	FindByKey(ctx context.Context, key string) (model.Feature, error)
}

type PlanEntitlementService struct {
	store        PlanEntitlementStore
	featureStore PlanEntitlementFeatureStore
}

func NewPlanEntitlementService(
	store PlanEntitlementStore,
	featureStore PlanEntitlementFeatureStore,
) *PlanEntitlementService {
	return &PlanEntitlementService{
		store:        store,
		featureStore: featureStore,
	}
}

func (s *PlanEntitlementService) ListByPlanID(ctx context.Context, planID string) (dto.PlanEntitlementListResponse, error) {
	entitlements, err := s.store.ListByPlanID(ctx, strings.TrimSpace(planID))
	if err != nil {
		return dto.PlanEntitlementListResponse{}, err
	}
	return dto.PlanEntitlementListResponse{Items: planEntitlementResponses(entitlements)}, nil
}

func (s *PlanEntitlementService) ReplaceByPlanID(
	ctx context.Context,
	planID string,
	request dto.ReplacePlanEntitlementsRequest,
) (dto.PlanEntitlementListResponse, error) {
	params := make([]repository.UpsertPlanEntitlementParams, 0, len(request.Entitlements))
	seen := map[string]struct{}{}
	for _, entitlement := range request.Entitlements {
		featureKey := strings.TrimSpace(entitlement.FeatureKey)
		if featureKey == "" {
			return dto.PlanEntitlementListResponse{}, validationError("feature key is required")
		}
		if _, exists := seen[featureKey]; exists {
			return dto.PlanEntitlementListResponse{}, validationError("feature key is duplicated")
		}
		seen[featureKey] = struct{}{}

		feature, err := s.featureStore.FindByKey(ctx, featureKey)
		if err != nil {
			return dto.PlanEntitlementListResponse{}, mapFeatureError(err)
		}
		if !feature.IsActive {
			return dto.PlanEntitlementListResponse{}, validationError("feature is inactive")
		}
		if err := validateEntitlementValue(feature, entitlement); err != nil {
			return dto.PlanEntitlementListResponse{}, err
		}
		params = append(params, repository.UpsertPlanEntitlementParams{
			FeatureID:    feature.ID,
			ValueBool:    entitlement.ValueBool,
			ValueInt:     entitlement.ValueInt,
			ValueDecimal: trimOptionalString(entitlement.ValueDecimal),
			ValueString:  trimOptionalString(entitlement.ValueString),
			Limits:       entitlement.Limits,
		})
	}
	entitlements, err := s.store.ReplaceByPlanID(ctx, strings.TrimSpace(planID), params)
	if err != nil {
		return dto.PlanEntitlementListResponse{}, err
	}
	return dto.PlanEntitlementListResponse{Items: planEntitlementResponses(entitlements)}, nil
}

func validateEntitlementValue(feature model.Feature, entitlement dto.PlanEntitlementValueRequest) error {
	if countEntitlementValues(entitlement) != 1 {
		return validationError("entitlement must contain exactly one value")
	}
	switch feature.ValueType {
	case model.FeatureValueTypeBoolean:
		if entitlement.ValueBool == nil {
			return validationError("boolean feature requires value_bool")
		}
	case model.FeatureValueTypeInteger:
		if entitlement.ValueInt == nil {
			return validationError("integer feature requires value_int")
		}
	case model.FeatureValueTypeDecimal:
		if entitlement.ValueDecimal == nil || strings.TrimSpace(*entitlement.ValueDecimal) == "" {
			return validationError("decimal feature requires value_decimal")
		}
	case model.FeatureValueTypeString:
		if entitlement.ValueString == nil || strings.TrimSpace(*entitlement.ValueString) == "" {
			return validationError("string feature requires value_string")
		}
	default:
		return validationError("feature value type is invalid")
	}
	return nil
}

func countEntitlementValues(entitlement dto.PlanEntitlementValueRequest) int {
	count := 0
	if entitlement.ValueBool != nil {
		count++
	}
	if entitlement.ValueInt != nil {
		count++
	}
	if entitlement.ValueDecimal != nil {
		count++
	}
	if entitlement.ValueString != nil {
		count++
	}
	return count
}
