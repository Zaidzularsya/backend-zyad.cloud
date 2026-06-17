package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type BrandingService interface {
	UpsertDefault(ctx context.Context, scope coretenant.Scope, params repository.CreateBrandingParams) (domain.LandingBranding, error)
	UpsertPageOverride(ctx context.Context, scope coretenant.Scope, pageID string, params repository.CreateBrandingParams) (domain.LandingBranding, error)
	GetEffectiveBranding(ctx context.Context, scope coretenant.Scope, pageID string) (domain.LandingBranding, error)
	RemovePageOverride(ctx context.Context, scope coretenant.Scope, pageID string) error
}
