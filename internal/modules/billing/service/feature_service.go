package service

import (
	"context"
	"strings"

	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/modules/billing/repository"
)

type FeatureStore interface {
	Create(ctx context.Context, params repository.CreateFeatureParams) (model.Feature, error)
	FindByID(ctx context.Context, id string) (model.Feature, error)
	FindByKey(ctx context.Context, key string) (model.Feature, error)
	List(ctx context.Context, filter repository.FeatureListFilter) ([]model.Feature, int64, error)
	Update(ctx context.Context, params repository.UpdateFeatureParams) (model.Feature, error)
}

type FeatureService struct {
	store FeatureStore
}

func NewFeatureService(store FeatureStore) *FeatureService {
	return &FeatureService{store: store}
}

func (s *FeatureService) List(ctx context.Context, query dto.FeatureListQuery) (dto.FeatureListResponse, error) {
	page, perPage := normalizePage(query.Page, query.PerPage)
	features, total, err := s.store.List(ctx, repository.FeatureListFilter{
		Module:   strings.TrimSpace(query.Module),
		IsActive: query.IsActive,
		Search:   strings.TrimSpace(query.Search),
		Limit:    perPage,
		Offset:   (page - 1) * perPage,
	})
	if err != nil {
		return dto.FeatureListResponse{}, err
	}
	return dto.FeatureListResponse{
		Items: featureResponses(features),
		Meta:  paginationMeta(page, perPage, total),
	}, nil
}

func (s *FeatureService) FindByID(ctx context.Context, id string) (dto.FeatureResponse, error) {
	feature, err := s.store.FindByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return dto.FeatureResponse{}, mapFeatureError(err)
	}
	return featureResponse(feature), nil
}

func (s *FeatureService) FindByKey(ctx context.Context, key string) (dto.FeatureResponse, error) {
	feature, err := s.store.FindByKey(ctx, strings.TrimSpace(key))
	if err != nil {
		return dto.FeatureResponse{}, mapFeatureError(err)
	}
	return featureResponse(feature), nil
}

func (s *FeatureService) Create(ctx context.Context, request dto.CreateFeatureRequest) (dto.FeatureResponse, error) {
	valueType := model.FeatureValueType(strings.TrimSpace(request.ValueType))
	if !valueType.IsValid() {
		return dto.FeatureResponse{}, validationError("feature value type is invalid")
	}
	resetStrategy := model.ResetStrategy(strings.TrimSpace(request.ResetStrategy))
	if resetStrategy == "" {
		resetStrategy = model.ResetStrategyNever
	}
	if !resetStrategy.IsValid() {
		return dto.FeatureResponse{}, validationError("feature reset strategy is invalid")
	}
	if strings.TrimSpace(request.FeatureKey) == "" {
		return dto.FeatureResponse{}, validationError("feature key is required")
	}
	if strings.TrimSpace(request.Module) == "" {
		return dto.FeatureResponse{}, validationError("feature module is required")
	}
	if strings.TrimSpace(request.Name) == "" {
		return dto.FeatureResponse{}, validationError("feature name is required")
	}

	feature, err := s.store.Create(ctx, repository.CreateFeatureParams{
		Key:           request.FeatureKey,
		Module:        request.Module,
		Name:          request.Name,
		Description:   request.Description,
		ValueType:     valueType,
		Unit:          request.Unit,
		ResetStrategy: resetStrategy,
		IsActive:      boolDefault(request.IsActive, true),
	})
	if err != nil {
		return dto.FeatureResponse{}, err
	}
	return featureResponse(feature), nil
}

func (s *FeatureService) Update(ctx context.Context, id string, request dto.UpdateFeatureRequest) (dto.FeatureResponse, error) {
	params := repository.UpdateFeatureParams{
		ID:          strings.TrimSpace(id),
		Module:      trimOptionalString(request.Module),
		Name:        trimOptionalString(request.Name),
		Description: trimOptionalString(request.Description),
		Unit:        trimOptionalString(request.Unit),
		IsActive:    request.IsActive,
	}
	if request.ValueType != nil {
		valueType := model.FeatureValueType(strings.TrimSpace(*request.ValueType))
		if !valueType.IsValid() {
			return dto.FeatureResponse{}, validationError("feature value type is invalid")
		}
		params.ValueType = &valueType
	}
	if request.ResetStrategy != nil {
		resetStrategy := model.ResetStrategy(strings.TrimSpace(*request.ResetStrategy))
		if !resetStrategy.IsValid() {
			return dto.FeatureResponse{}, validationError("feature reset strategy is invalid")
		}
		params.ResetStrategy = &resetStrategy
	}
	feature, err := s.store.Update(ctx, params)
	if err != nil {
		return dto.FeatureResponse{}, mapFeatureError(err)
	}
	return featureResponse(feature), nil
}
