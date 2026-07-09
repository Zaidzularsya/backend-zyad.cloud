package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type PageListFilter struct {
	Status         domain.PageStatus
	PageType       domain.PageType
	IsTemplate     *bool
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// CreatePageParams intentionally excludes OrganizationID. Implementations must
// persist organization identity from Scope.
type CreatePageParams struct {
	Name       string
	Title      string
	Slug       string
	Type       domain.PageType
	Status     domain.PageStatus
	Visibility domain.PageVisibility
	SEO        map[string]any
	Settings   *domain.PageSettings
	Locale     string
	Timezone   string
	IsHomepage bool
	IsTemplate bool
	CreatedBy  string
}

type UpdatePageParams struct {
	Name         *string
	Title        *string
	Slug         *string
	Type         *domain.PageType
	Status       *domain.PageStatus
	Visibility   *domain.PageVisibility
	PasswordHash *string
	SEO          map[string]any
	Settings     *domain.PageSettings
	Locale       *string
	Timezone     *string
	IsHomepage   *bool
	IsTemplate   *bool
	UpdatedBy    string
}

// PageRepository is the tenant-owned data contract. Every method requires a
// verified immutable scope and row lookups include both scope and resource ID.
type PageRepository interface {
	Create(context.Context, coretenant.Scope, CreatePageParams) (domain.LandingPage, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.LandingPage, error)
	FindBySlug(context.Context, coretenant.Scope, string) (domain.LandingPage, error)
	List(context.Context, coretenant.Scope, PageListFilter) ([]domain.LandingPage, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdatePageParams) (domain.LandingPage, error)
	Delete(context.Context, coretenant.Scope, string) error
}

// PlatformPageRepository is the explicit cross-tenant boundary. Platform
// handlers must never satisfy tenant PageRepository through this interface.
type PlatformPageRepository interface {
	FindByOrganizationAndID(
		context.Context,
		string,
		string,
	) (domain.LandingPage, error)
	ListByOrganization(
		context.Context,
		string,
		PageListFilter,
	) ([]domain.LandingPage, int64, error)
}
