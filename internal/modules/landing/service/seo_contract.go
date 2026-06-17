package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type SeoService interface {
	UpdatePageSEO(ctx context.Context, scope coretenant.Scope, pageID string, seoData map[string]any) (domain.LandingPage, error)
	GetEffectiveSEO(ctx context.Context, scope coretenant.Scope, pageID string) (map[string]any, error)
}
