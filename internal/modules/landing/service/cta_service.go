package service

import (
	"context"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type ctaService struct {
	reusableRepo repository.ReusableRepository
}

func NewCTAService(reusableRepo repository.ReusableRepository) CTAService {
	return &ctaService{
		reusableRepo: reusableRepo,
	}
}

func (s *ctaService) Create(ctx context.Context, scope tenant.Scope, params repository.CreateCTAParams) (domain.LandingCTA, error) {
	if !scope.IsValid() {
		return domain.LandingCTA{}, tenant.ErrInvalidScope
	}

	if !params.Type.IsValid() {
		return domain.LandingCTA{}, ErrInvalidCTAType
	}

	if !params.Target.IsValid() {
		return domain.LandingCTA{}, ErrInvalidTarget
	}

	return s.reusableRepo.CreateCTA(ctx, scope, params)
}

func (s *ctaService) FindByID(ctx context.Context, scope tenant.Scope, id string) (domain.LandingCTA, error) {
	if !scope.IsValid() {
		return domain.LandingCTA{}, tenant.ErrInvalidScope
	}

	return s.reusableRepo.GetCTA(ctx, scope, id)
}

func (s *ctaService) List(ctx context.Context, scope tenant.Scope) ([]domain.LandingCTA, error) {
	if !scope.IsValid() {
		return nil, tenant.ErrInvalidScope
	}

	return s.reusableRepo.ListCTAs(ctx, scope)
}

func (s *ctaService) Update(ctx context.Context, scope tenant.Scope, id string, params repository.UpdateCTAParams) (domain.LandingCTA, error) {
	if !scope.IsValid() {
		return domain.LandingCTA{}, tenant.ErrInvalidScope
	}

	if params.Type != nil && !params.Type.IsValid() {
		return domain.LandingCTA{}, ErrInvalidCTAType
	}

	if params.Target != nil && !params.Target.IsValid() {
		return domain.LandingCTA{}, ErrInvalidTarget
	}

	return s.reusableRepo.UpdateCTA(ctx, scope, id, params)
}

func (s *ctaService) Delete(ctx context.Context, scope tenant.Scope, id string, updatedBy string) error {
	if !scope.IsValid() {
		return tenant.ErrInvalidScope
	}

	return s.reusableRepo.DeleteCTA(ctx, scope, id, updatedBy)
}
