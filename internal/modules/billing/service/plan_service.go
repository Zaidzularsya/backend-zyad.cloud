package service

import (
	"context"
	"strings"

	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/modules/billing/repository"
)

type PlanStore interface {
	Create(ctx context.Context, params repository.CreatePlanParams) (model.Plan, error)
	FindByID(ctx context.Context, id string) (model.Plan, error)
	FindByCode(ctx context.Context, code string) (model.Plan, error)
	List(ctx context.Context, filter repository.PlanListFilter) ([]model.Plan, int64, error)
	Update(ctx context.Context, params repository.UpdatePlanParams) (model.Plan, error)
	SoftDelete(ctx context.Context, id string) error
	UpsertPrice(ctx context.Context, params repository.UpsertPlanPriceParams) (model.PlanPrice, error)
	CreatePrice(ctx context.Context, params repository.CreatePlanPriceParams) (model.PlanPrice, error)
	ListPrices(ctx context.Context, planID string, includeDeleted bool) ([]model.PlanPrice, error)
	FindPriceByID(ctx context.Context, planID string, priceID string, includeDeleted bool) (model.PlanPrice, error)
	UpdatePrice(ctx context.Context, params repository.UpdatePlanPriceParams) (model.PlanPrice, error)
	SoftDeletePrice(ctx context.Context, planID string, priceID string) error
}

type PlanService struct {
	store PlanStore
}

func NewPlanService(store PlanStore) *PlanService {
	return &PlanService{store: store}
}

func (s *PlanService) List(ctx context.Context, query dto.PlanListQuery) (dto.PlanListResponse, error) {
	filter, page, perPage, err := planListFilter(query)
	if err != nil {
		return dto.PlanListResponse{}, err
	}
	plans, total, err := s.store.List(ctx, filter)
	if err != nil {
		return dto.PlanListResponse{}, err
	}
	items := make([]dto.PlanResponse, 0, len(plans))
	for _, plan := range plans {
		items = append(items, planResponse(plan, nil))
	}
	return dto.PlanListResponse{
		Items: items,
		Meta:  paginationMeta(page, perPage, total),
	}, nil
}

func (s *PlanService) FindByID(ctx context.Context, id string, includePrices bool) (dto.PlanResponse, error) {
	plan, err := s.store.FindByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return dto.PlanResponse{}, mapPlanError(err)
	}
	var prices []model.PlanPrice
	if includePrices {
		prices, err = s.store.ListPrices(ctx, plan.ID, false)
		if err != nil {
			return dto.PlanResponse{}, err
		}
	}
	return planResponse(plan, prices), nil
}

func (s *PlanService) FindByCode(ctx context.Context, code string, includePrices bool) (dto.PlanResponse, error) {
	plan, err := s.store.FindByCode(ctx, strings.TrimSpace(code))
	if err != nil {
		return dto.PlanResponse{}, mapPlanError(err)
	}
	var prices []model.PlanPrice
	if includePrices {
		prices, err = s.store.ListPrices(ctx, plan.ID, false)
		if err != nil {
			return dto.PlanResponse{}, err
		}
	}
	return planResponse(plan, prices), nil
}

func (s *PlanService) Create(ctx context.Context, request dto.CreatePlanRequest) (dto.PlanResponse, error) {
	planType := model.PlanType(strings.TrimSpace(request.Type))
	if !planType.IsValid() {
		return dto.PlanResponse{}, validationError("plan type is invalid")
	}
	if strings.TrimSpace(request.Code) == "" {
		return dto.PlanResponse{}, validationError("plan code is required")
	}
	if strings.TrimSpace(request.Name) == "" {
		return dto.PlanResponse{}, validationError("plan name is required")
	}
	for _, price := range request.Prices {
		if err := validatePriceRequest(price.BillingInterval, price.Amount); err != nil {
			return dto.PlanResponse{}, err
		}
	}

	isPublic := boolDefault(request.IsPublic, true)
	isActive := boolDefault(request.IsActive, true)
	plan, err := s.store.Create(ctx, repository.CreatePlanParams{
		Code:        request.Code,
		Name:        request.Name,
		Description: request.Description,
		Type:        planType,
		IsPublic:    isPublic,
		IsActive:    isActive,
		SortOrder:   request.SortOrder,
		Metadata:    request.Metadata,
	})
	if err != nil {
		return dto.PlanResponse{}, err
	}

	prices := make([]model.PlanPrice, 0, len(request.Prices))
	for _, priceRequest := range request.Prices {
		price, err := s.upsertPrice(ctx, plan.ID, priceRequest)
		if err != nil {
			return dto.PlanResponse{}, err
		}
		prices = append(prices, price)
	}
	return planResponse(plan, prices), nil
}

func (s *PlanService) Update(ctx context.Context, id string, request dto.UpdatePlanRequest) (dto.PlanResponse, error) {
	params := repository.UpdatePlanParams{
		ID:          strings.TrimSpace(id),
		Name:        trimOptionalString(request.Name),
		Description: trimOptionalString(request.Description),
		IsPublic:    request.IsPublic,
		IsActive:    request.IsActive,
		SortOrder:   request.SortOrder,
		Metadata:    request.Metadata,
	}
	if request.Type != nil {
		planType := model.PlanType(strings.TrimSpace(*request.Type))
		if !planType.IsValid() {
			return dto.PlanResponse{}, validationError("plan type is invalid")
		}
		params.Type = &planType
	}
	plan, err := s.store.Update(ctx, params)
	if err != nil {
		return dto.PlanResponse{}, mapPlanError(err)
	}
	return planResponse(plan, nil), nil
}

func (s *PlanService) Delete(ctx context.Context, id string) error {
	planID := strings.TrimSpace(id)
	if _, err := s.store.FindByID(ctx, planID); err != nil {
		return mapPlanError(err)
	}
	return s.store.SoftDelete(ctx, planID)
}

func (s *PlanService) UpsertPrice(ctx context.Context, planID string, request dto.UpsertPlanPriceRequest) (dto.PlanPriceResponse, error) {
	price, err := s.upsertPrice(ctx, strings.TrimSpace(planID), dto.PlanPriceRequest(request))
	if err != nil {
		return dto.PlanPriceResponse{}, err
	}
	return planPriceResponse(price), nil
}

func (s *PlanService) ListPrices(ctx context.Context, planID string, includeDeleted bool) ([]dto.PlanPriceResponse, error) {
	trimmedPlanID := strings.TrimSpace(planID)
	if _, err := s.store.FindByID(ctx, trimmedPlanID); err != nil {
		return nil, mapPlanError(err)
	}
	prices, err := s.store.ListPrices(ctx, trimmedPlanID, includeDeleted)
	if err != nil {
		return nil, err
	}
	return planPriceResponses(prices), nil
}

func (s *PlanService) CreatePrice(
	ctx context.Context,
	planID string,
	request dto.CreatePlanPriceRequest,
) (dto.PlanPriceResponse, error) {
	trimmedPlanID := strings.TrimSpace(planID)
	if _, err := s.store.FindByID(ctx, trimmedPlanID); err != nil {
		return dto.PlanPriceResponse{}, mapPlanError(err)
	}
	if err := validatePriceRequest(request.BillingInterval, request.Amount); err != nil {
		return dto.PlanPriceResponse{}, err
	}

	price, err := s.store.CreatePrice(ctx, repository.CreatePlanPriceParams{
		PlanID:          trimmedPlanID,
		BillingInterval: model.BillingInterval(strings.TrimSpace(request.BillingInterval)),
		Currency:        request.Currency,
		Amount:          request.Amount,
		IsActive:        boolDefault(request.IsActive, true),
		Metadata:        request.Metadata,
	})
	if err != nil {
		return dto.PlanPriceResponse{}, err
	}
	return planPriceResponse(price), nil
}

func (s *PlanService) UpdatePrice(
	ctx context.Context,
	planID string,
	priceID string,
	request dto.UpdatePlanPriceRequest,
) (dto.PlanPriceResponse, error) {
	trimmedPlanID := strings.TrimSpace(planID)
	trimmedPriceID := strings.TrimSpace(priceID)
	if _, err := s.store.FindByID(ctx, trimmedPlanID); err != nil {
		return dto.PlanPriceResponse{}, mapPlanError(err)
	}
	if _, err := s.store.FindPriceByID(ctx, trimmedPlanID, trimmedPriceID, false); err != nil {
		return dto.PlanPriceResponse{}, mapPlanPriceError(err)
	}

	params := repository.UpdatePlanPriceParams{
		ID:       trimmedPriceID,
		PlanID:   trimmedPlanID,
		IsActive: request.IsActive,
		Metadata: request.Metadata,
	}
	if request.BillingInterval != nil {
		interval := model.BillingInterval(strings.TrimSpace(*request.BillingInterval))
		if !interval.IsValidPlanPriceInterval() {
			return dto.PlanPriceResponse{}, validationError("billing interval is invalid")
		}
		params.BillingInterval = &interval
	}
	if request.Amount != nil {
		if strings.TrimSpace(*request.Amount) == "" {
			return dto.PlanPriceResponse{}, validationError("price amount is required")
		}
		params.Amount = request.Amount
	}
	if request.Currency != nil {
		params.Currency = request.Currency
	}

	price, err := s.store.UpdatePrice(ctx, params)
	if err != nil {
		return dto.PlanPriceResponse{}, mapPlanPriceError(err)
	}
	return planPriceResponse(price), nil
}

func (s *PlanService) DeletePrice(ctx context.Context, planID string, priceID string) error {
	trimmedPlanID := strings.TrimSpace(planID)
	trimmedPriceID := strings.TrimSpace(priceID)
	if _, err := s.store.FindByID(ctx, trimmedPlanID); err != nil {
		return mapPlanError(err)
	}
	if _, err := s.store.FindPriceByID(ctx, trimmedPlanID, trimmedPriceID, false); err != nil {
		return mapPlanPriceError(err)
	}
	if err := s.store.SoftDeletePrice(ctx, trimmedPlanID, trimmedPriceID); err != nil {
		return mapPlanPriceError(err)
	}
	return nil
}

func (s *PlanService) upsertPrice(ctx context.Context, planID string, request dto.PlanPriceRequest) (model.PlanPrice, error) {
	if err := validatePriceRequest(request.BillingInterval, request.Amount); err != nil {
		return model.PlanPrice{}, err
	}
	interval := model.BillingInterval(strings.TrimSpace(request.BillingInterval))
	return s.store.UpsertPrice(ctx, repository.UpsertPlanPriceParams{
		PlanID:          strings.TrimSpace(planID),
		BillingInterval: interval,
		Currency:        request.Currency,
		Amount:          request.Amount,
		IsActive:        boolDefault(request.IsActive, true),
		Metadata:        request.Metadata,
	})
}

func planListFilter(query dto.PlanListQuery) (repository.PlanListFilter, int, int, error) {
	page, perPage := normalizePage(query.Page, query.PerPage)
	filter := repository.PlanListFilter{
		IsPublic:       query.IsPublic,
		IsActive:       query.IsActive,
		IncludeDeleted: query.IncludeDeleted,
		Search:         strings.TrimSpace(query.Search),
		Limit:          perPage,
		Offset:         (page - 1) * perPage,
	}
	if strings.TrimSpace(query.Type) != "" {
		planType := model.PlanType(strings.TrimSpace(query.Type))
		if !planType.IsValid() {
			return repository.PlanListFilter{}, 0, 0, validationError("plan type is invalid")
		}
		filter.Type = planType
	}
	return filter, page, perPage, nil
}

func validatePriceRequest(intervalValue string, amount string) error {
	interval := model.BillingInterval(strings.TrimSpace(intervalValue))
	if !interval.IsValidPlanPriceInterval() {
		return validationError("billing interval is invalid")
	}
	if strings.TrimSpace(amount) == "" {
		return validationError("price amount is required")
	}
	return nil
}
