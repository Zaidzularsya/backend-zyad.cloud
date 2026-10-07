package service

import (
	"zyad.cloud/internal/modules/product/dto"
	"zyad.cloud/internal/modules/product/model"
)

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
