package service

import (
	"context"
	"errors"
	"strings"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

var (
	ErrPricingPlanNameRequired  = errors.New("pricing plan name is required")
	ErrPricingPlanPriceRequired = errors.New("pricing plan price label is required")
)

type pricingService struct {
	reusableRepo repository.ReusableRepository
}

func NewPricingService(reusableRepo repository.ReusableRepository) PricingService {
	return &pricingService{
		reusableRepo: reusableRepo,
	}
}

func (s *pricingService) Create(ctx context.Context, scope tenant.Scope, params repository.CreatePricingPlanParams) (domain.LandingPricingPlan, error) {
	if !scope.IsValid() {
		return domain.LandingPricingPlan{}, tenant.ErrInvalidScope
	}
	if strings.TrimSpace(params.Name) == "" {
		return domain.LandingPricingPlan{}, ErrPricingPlanNameRequired
	}
	if strings.TrimSpace(params.PriceLabel) == "" {
		return domain.LandingPricingPlan{}, ErrPricingPlanPriceRequired
	}
	if params.CTALabel == "" {
		params.CTALabel = "Pilih paket"
	}

	return s.reusableRepo.CreatePricingPlan(ctx, scope, params)
}

func (s *pricingService) FindByID(ctx context.Context, scope tenant.Scope, id string) (domain.LandingPricingPlan, error) {
	if !scope.IsValid() {
		return domain.LandingPricingPlan{}, tenant.ErrInvalidScope
	}

	return s.reusableRepo.GetPricingPlan(ctx, scope, id)
}

func (s *pricingService) List(ctx context.Context, scope tenant.Scope) ([]domain.LandingPricingPlan, error) {
	if !scope.IsValid() {
		return nil, tenant.ErrInvalidScope
	}

	return s.reusableRepo.ListPricingPlans(ctx, scope)
}

func (s *pricingService) Update(ctx context.Context, scope tenant.Scope, id string, params repository.UpdatePricingPlanParams) (domain.LandingPricingPlan, error) {
	if !scope.IsValid() {
		return domain.LandingPricingPlan{}, tenant.ErrInvalidScope
	}
	if params.Name != nil && strings.TrimSpace(*params.Name) == "" {
		return domain.LandingPricingPlan{}, ErrPricingPlanNameRequired
	}
	if params.PriceLabel != nil && strings.TrimSpace(*params.PriceLabel) == "" {
		return domain.LandingPricingPlan{}, ErrPricingPlanPriceRequired
	}

	return s.reusableRepo.UpdatePricingPlan(ctx, scope, id, params)
}

func (s *pricingService) Reorder(ctx context.Context, scope tenant.Scope, planIDs []string) error {
	if !scope.IsValid() {
		return tenant.ErrInvalidScope
	}

	return s.reusableRepo.ReorderPricingPlans(ctx, scope, planIDs)
}

func (s *pricingService) Delete(ctx context.Context, scope tenant.Scope, id string, updatedBy string) error {
	if !scope.IsValid() {
		return tenant.ErrInvalidScope
	}

	return s.reusableRepo.DeletePricingPlan(ctx, scope, id, updatedBy)
}
