package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/repository"
)

type CRMSettingsService interface {
	Get(context.Context, coretenant.Scope) (repository.CRMSettings, error)
	Update(ctx context.Context, scope coretenant.Scope, leadPlaybookEnabled bool, updatedBy string) (repository.CRMSettings, error)
}

type crmSettingsService struct {
	repo repository.CRMSettingsRepository
}

func NewCRMSettingsService(repo repository.CRMSettingsRepository) CRMSettingsService {
	return &crmSettingsService{repo: repo}
}

func (s *crmSettingsService) Get(ctx context.Context, scope coretenant.Scope) (repository.CRMSettings, error) {
	return s.repo.Get(ctx, scope)
}

func (s *crmSettingsService) Update(ctx context.Context, scope coretenant.Scope, enabled bool, updatedBy string) (repository.CRMSettings, error) {
	return s.repo.Update(ctx, scope, enabled, updatedBy)
}
