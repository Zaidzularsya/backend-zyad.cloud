package service

import (
	"strconv"
	"strings"

	"zyad.cloud/internal/modules/product/dto"
	"zyad.cloud/internal/modules/product/model"
	"zyad.cloud/internal/modules/product/repository"
)

func planResponse(plan model.Plan, prices []model.PlanPrice) dto.PlanResponse {
	return dto.PlanResponse{
		ID:          plan.ID,
		Code:        plan.Code,
		Name:        plan.Name,
		Description: plan.Description,
		PlanType:    string(plan.Type),
		IsPublic:    plan.IsPublic,
		IsActive:    plan.IsActive,
		SortOrder:   plan.SortOrder,
		Metadata:    mapOrEmpty(plan.Metadata),
		Prices:      planPriceResponses(prices),
		CreatedAt:   formatTime(plan.CreatedAt),
		UpdatedAt:   formatTime(plan.UpdatedAt),
		DeletedAt:   formatOptionalTime(plan.DeletedAt),
	}
}

func publicPlanSummary(
	plan model.Plan,
	prices []model.PlanPrice,
	entitlements []repository.PublicEntitlement,
) dto.PublicPlanSummary {
	items := make([]dto.PublicPlanPriceSummary, 0, len(prices))
	for _, price := range prices {
		if !price.IsAvailable() {
			continue
		}
		items = append(items, dto.PublicPlanPriceSummary{
			BillingInterval: string(price.BillingInterval),
			Currency:        price.Currency,
			Amount:          price.Amount,
		})
	}
	return dto.PublicPlanSummary{
		ID:          plan.ID,
		Code:        plan.Code,
		Name:        plan.Name,
		Description: plan.Description,
		SortOrder:   plan.SortOrder,
		Prices:      items,
		Benefits:    publicPlanBenefits(entitlements),
	}
}

func publicPlanBenefits(entitlements []repository.PublicEntitlement) []dto.PublicPlanBenefit {
	items := make([]dto.PublicPlanBenefit, 0, len(entitlements))
	for _, entitlement := range entitlements {
		switch entitlement.ValueType {
		case model.FeatureValueTypeBoolean:
			if entitlement.ValueBool == nil || !*entitlement.ValueBool {
				continue
			}
			items = append(items, dto.PublicPlanBenefit{Label: entitlement.FeatureName})
		case model.FeatureValueTypeInteger:
			if entitlement.ValueInt == nil {
				continue
			}
			items = append(items, dto.PublicPlanBenefit{
				Label: entitlement.FeatureName,
				Value: withUnit(strconv.FormatInt(*entitlement.ValueInt, 10), entitlement.Unit),
			})
		case model.FeatureValueTypeDecimal:
			if entitlement.ValueDecimal == nil || strings.TrimSpace(*entitlement.ValueDecimal) == "" {
				continue
			}
			items = append(items, dto.PublicPlanBenefit{
				Label: entitlement.FeatureName,
				Value: withUnit(strings.TrimSpace(*entitlement.ValueDecimal), entitlement.Unit),
			})
		case model.FeatureValueTypeString:
			if entitlement.ValueString == nil || strings.TrimSpace(*entitlement.ValueString) == "" {
				continue
			}
			items = append(items, dto.PublicPlanBenefit{
				Label: entitlement.FeatureName,
				Value: strings.TrimSpace(*entitlement.ValueString),
			})
		}
	}
	return items
}

func withUnit(value string, unit string) string {
	if strings.TrimSpace(unit) == "" {
		return value
	}
	return value + " " + strings.TrimSpace(unit)
}

func planPriceResponse(price model.PlanPrice) dto.PlanPriceResponse {
	return dto.PlanPriceResponse{
		ID:              price.ID,
		PlanID:          price.PlanID,
		BillingInterval: string(price.BillingInterval),
		Currency:        price.Currency,
		Amount:          price.Amount,
		IsActive:        price.IsActive,
		Metadata:        mapOrEmpty(price.Metadata),
		CreatedAt:       formatTime(price.CreatedAt),
		UpdatedAt:       formatTime(price.UpdatedAt),
		DeletedAt:       formatOptionalTime(price.DeletedAt),
	}
}

func planPriceResponses(prices []model.PlanPrice) []dto.PlanPriceResponse {
	items := make([]dto.PlanPriceResponse, 0, len(prices))
	for _, price := range prices {
		items = append(items, planPriceResponse(price))
	}
	return items
}

func featureResponse(feature model.Feature) dto.FeatureResponse {
	return dto.FeatureResponse{
		ID:            feature.ID,
		FeatureKey:    feature.Key,
		Module:        feature.Module,
		Name:          feature.Name,
		Description:   feature.Description,
		ValueType:     string(feature.ValueType),
		Unit:          feature.Unit,
		ResetStrategy: string(feature.ResetStrategy),
		IsActive:      feature.IsActive,
		CreatedAt:     formatTime(feature.CreatedAt),
		UpdatedAt:     formatTime(feature.UpdatedAt),
	}
}

func featureResponses(features []model.Feature) []dto.FeatureResponse {
	items := make([]dto.FeatureResponse, 0, len(features))
	for _, feature := range features {
		items = append(items, featureResponse(feature))
	}
	return items
}

func planEntitlementResponse(entitlement model.PlanEntitlement) dto.PlanEntitlementResponse {
	return dto.PlanEntitlementResponse{
		ID:           entitlement.ID,
		PlanID:       entitlement.PlanID,
		FeatureID:    entitlement.FeatureID,
		FeatureKey:   entitlement.FeatureKey,
		ValueBool:    entitlement.ValueBool,
		ValueInt:     entitlement.ValueInt,
		ValueDecimal: entitlement.ValueDecimal,
		ValueString:  entitlement.ValueString,
		Limits:       mapOrEmpty(entitlement.Limits),
		CreatedAt:    formatTime(entitlement.CreatedAt),
		UpdatedAt:    formatTime(entitlement.UpdatedAt),
	}
}

func planEntitlementResponses(entitlements []model.PlanEntitlement) []dto.PlanEntitlementResponse {
	items := make([]dto.PlanEntitlementResponse, 0, len(entitlements))
	for _, entitlement := range entitlements {
		items = append(items, planEntitlementResponse(entitlement))
	}
	return items
}
