package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type DuplicatePageParams struct {
	PageID string
	UserID string
}

// InstantiatePageFromTemplateParams creates a brand new page seeded from an
// existing page-level template (a LandingPage with IsTemplate=true), copying
// its sections, SEO, and optionally its per-page branding override in one call.
type InstantiatePageFromTemplateParams struct {
	TemplatePageID  string
	Name            string
	Title           string
	Slug            string
	Visibility      domain.PageVisibility
	Locale          string
	Timezone        string
	IncludeBranding bool
	CreatedBy       string
}

type PageService interface {
	Create(context.Context, coretenant.Scope, repository.CreatePageParams) (domain.LandingPage, error)
	Get(context.Context, coretenant.Scope, string) (domain.LandingPage, error)
	List(context.Context, coretenant.Scope, repository.PageListFilter) ([]domain.LandingPage, int64, error)
	Update(context.Context, coretenant.Scope, string, repository.UpdatePageParams) (domain.LandingPage, error)
	Delete(context.Context, coretenant.Scope, string) error
	Duplicate(context.Context, coretenant.Scope, DuplicatePageParams) (domain.LandingPage, error)
	InstantiateFromTemplate(context.Context, coretenant.Scope, InstantiatePageFromTemplateParams) (domain.LandingPage, error)
	Archive(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
}
