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

type PageService interface {
	Create(context.Context, coretenant.Scope, repository.CreatePageParams) (domain.LandingPage, error)
	Get(context.Context, coretenant.Scope, string) (domain.LandingPage, error)
	List(context.Context, coretenant.Scope, repository.PageListFilter) ([]domain.LandingPage, int64, error)
	Update(context.Context, coretenant.Scope, string, repository.UpdatePageParams) (domain.LandingPage, error)
	Delete(context.Context, coretenant.Scope, string) error
	Duplicate(context.Context, coretenant.Scope, DuplicatePageParams) (domain.LandingPage, error)
	Archive(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
}
