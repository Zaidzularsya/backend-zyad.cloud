package service

import (
	"context"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

// PricingService manages a tenant's own pricing plan cards — marketing
// content for their landing pages, not the platform's own subscription
// plans (see internal/modules/product for that).
type PricingService interface {
	Create(ctx context.Context, scope tenant.Scope, params repository.CreatePricingPlanParams) (domain.LandingPricingPlan, error)
	FindByID(ctx context.Context, scope tenant.Scope, id string) (domain.LandingPricingPlan, error)
	List(ctx context.Context, scope tenant.Scope) ([]domain.LandingPricingPlan, error)
	Update(ctx context.Context, scope tenant.Scope, id string, params repository.UpdatePricingPlanParams) (domain.LandingPricingPlan, error)
	Reorder(ctx context.Context, scope tenant.Scope, planIDs []string) error
	Delete(ctx context.Context, scope tenant.Scope, id string, updatedBy string) error
}
