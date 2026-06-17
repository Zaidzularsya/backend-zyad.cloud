package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type CreateVersionParams struct {
	LandingPageID string
	Version       int
	ChangeNote    string
	Snapshot      map[string]any
	CreatedBy     string
}

type CreateSlugRedirectParams struct {
	SourceSlug string
	TargetSlug string
}

type VersionRepository interface {
	// Version Operations
	Create(context.Context, coretenant.Scope, CreateVersionParams) (domain.LandingPageVersion, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.LandingPageVersion, error)
	FindByVersion(context.Context, coretenant.Scope, string, int) (domain.LandingPageVersion, error)
	ListByPage(context.Context, coretenant.Scope, string) ([]domain.LandingPageVersion, error)

	// Slug Redirect Operations
	CreateRedirect(context.Context, coretenant.Scope, CreateSlugRedirectParams) (domain.LandingSlugRedirect, error)
	FindBySourceSlug(context.Context, coretenant.Scope, string) (domain.LandingSlugRedirect, error)
	ListRedirects(context.Context, coretenant.Scope) ([]domain.LandingSlugRedirect, error)
	UpdateRedirect(context.Context, coretenant.Scope, string, string) (domain.LandingSlugRedirect, error)
	DeleteRedirect(context.Context, coretenant.Scope, string) error
}
