package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type CompanyService interface {
	Create(context.Context, coretenant.Scope, repository.CreateCompanyParams) (domain.Company, error)
	Get(context.Context, coretenant.Scope, string) (domain.Company, error)
	List(context.Context, coretenant.Scope, repository.CompanyListFilter) ([]domain.Company, int64, error)
	Update(context.Context, coretenant.Scope, string, repository.UpdateCompanyParams) (domain.Company, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Restore(context.Context, coretenant.Scope, string, string) error
}
