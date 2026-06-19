package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type ResolverRepository interface {
	// ResolveBySlug finds a page by its slug
	ResolveBySlug(ctx context.Context, scope coretenant.Scope, slug string) (domain.LandingPage, error)

	// ResolveByDomain finds a page by a custom domain
	ResolveByDomain(ctx context.Context, scope coretenant.Scope, customDomain string) (domain.LandingPage, error)

	// Resolve public relations relying on RLS
	ResolveSections(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingSection, error)
	ResolveForms(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingForm, error)
	ResolveBranding(ctx context.Context, scope coretenant.Scope, pageID string) (domain.LandingBranding, error)
	ResolveVersions(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingPageVersion, error)
	ResolveMenus(ctx context.Context, scope coretenant.Scope) ([]domain.LandingMenu, error)
	ResolveMenuItems(ctx context.Context, scope coretenant.Scope, menuID string) ([]domain.LandingMenuItem, error)
}
