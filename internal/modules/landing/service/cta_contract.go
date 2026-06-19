package service

import (
	"context"

	"errors"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

var (
	ErrInvalidCTAType = errors.New("invalid cta type")
	ErrInvalidTarget  = errors.New("invalid cta target")
)

// CTAService interface defines operations for reusable Call to Actions
type CTAService interface {
	Create(ctx context.Context, scope tenant.Scope, params repository.CreateCTAParams) (domain.LandingCTA, error)
	FindByID(ctx context.Context, scope tenant.Scope, id string) (domain.LandingCTA, error)
	List(ctx context.Context, scope tenant.Scope) ([]domain.LandingCTA, error)
	Update(ctx context.Context, scope tenant.Scope, id string, params repository.UpdateCTAParams) (domain.LandingCTA, error)
	Delete(ctx context.Context, scope tenant.Scope, id string, updatedBy string) error
}
