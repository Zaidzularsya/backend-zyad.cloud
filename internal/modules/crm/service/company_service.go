package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type companyService struct {
	repo repository.CompanyRepository
}

func NewCompanyService(repo repository.CompanyRepository) CompanyService {
	return &companyService{repo: repo}
}

func (s *companyService) Create(ctx context.Context, scope coretenant.Scope, params repository.CreateCompanyParams) (domain.Company, error) {
	return s.repo.Create(ctx, scope, params)
}

func (s *companyService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Company, error) {
	company, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Company{}, crmmodule.MapNotFound(err, "COMPANY_NOT_FOUND", "company not found or already deleted")
	}
	return company, nil
}

func (s *companyService) List(ctx context.Context, scope coretenant.Scope, filter repository.CompanyListFilter) ([]domain.Company, int64, error) {
	return s.repo.List(ctx, scope, filter)
}

func (s *companyService) Update(ctx context.Context, scope coretenant.Scope, id string, params repository.UpdateCompanyParams) (domain.Company, error) {
	company, err := s.repo.Update(ctx, scope, id, params)
	if err != nil {
		return domain.Company{}, crmmodule.MapNotFound(err, "COMPANY_NOT_FOUND", "company not found or already deleted")
	}
	return company, nil
}

func (s *companyService) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	return crmmodule.MapNotFound(s.repo.Delete(ctx, scope, id, deletedBy), "COMPANY_NOT_FOUND", "company not found or already deleted")
}

func (s *companyService) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	return crmmodule.MapNotFound(s.repo.Restore(ctx, scope, id, restoredBy), "COMPANY_NOT_FOUND", "company not found or not deleted")
}
