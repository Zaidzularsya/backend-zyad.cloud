package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

// PipelineFeatureGate mirrors internal/modules/landing/service.DomainFeatureGate
// — a narrow interface over SubscriptionGuardService.RequireFeature.
type PipelineFeatureGate interface {
	RequireFeature(ctx context.Context, organizationID string, featureKey string) (organizationmodel.Entitlement, error)
}

type pipelineService struct {
	repo    repository.PipelineRepository
	feature PipelineFeatureGate
}

type PipelineServiceOption func(*pipelineService)

func WithPipelineFeatureGate(gate PipelineFeatureGate) PipelineServiceOption {
	return func(service *pipelineService) {
		service.feature = gate
	}
}

func NewPipelineService(repo repository.PipelineRepository, options ...PipelineServiceOption) PipelineService {
	service := &pipelineService{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

// Create allows the first pipeline for an organization for free — CRM needs
// at least one default pipeline for Deal to function even when the tenant's
// plan doesn't include the crm.pipeline feature. Only the 2nd+ pipeline is
// gated. See docs/reference-crm.md "Entitlement Enforcement".
func (s *pipelineService) Create(ctx context.Context, scope coretenant.Scope, params repository.CreatePipelineParams) (domain.Pipeline, error) {
	if s.feature != nil {
		activeCount, err := s.repo.CountActive(ctx, scope)
		if err != nil {
			return domain.Pipeline{}, err
		}
		if activeCount > 0 {
			if _, err := s.feature.RequireFeature(ctx, scope.OrganizationID(), domain.FeatureCRMPipeline); err != nil {
				return domain.Pipeline{}, err
			}
		}
	}
	return s.repo.Create(ctx, scope, params)
}

func (s *pipelineService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Pipeline, error) {
	pipeline, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Pipeline{}, crmmodule.MapNotFound(err, "PIPELINE_NOT_FOUND", "pipeline not found or already deleted")
	}
	return pipeline, nil
}

func (s *pipelineService) List(ctx context.Context, scope coretenant.Scope, filter repository.PipelineListFilter) ([]domain.Pipeline, int64, error) {
	return s.repo.List(ctx, scope, filter)
}

func (s *pipelineService) Update(ctx context.Context, scope coretenant.Scope, id string, params repository.UpdatePipelineParams) (domain.Pipeline, error) {
	pipeline, err := s.repo.Update(ctx, scope, id, params)
	if err != nil {
		return domain.Pipeline{}, crmmodule.MapNotFound(err, "PIPELINE_NOT_FOUND", "pipeline not found or already deleted")
	}
	return pipeline, nil
}

func (s *pipelineService) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	return crmmodule.MapNotFound(s.repo.Delete(ctx, scope, id, deletedBy), "PIPELINE_NOT_FOUND", "pipeline not found or already deleted")
}

func (s *pipelineService) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	return crmmodule.MapNotFound(s.repo.Restore(ctx, scope, id, restoredBy), "PIPELINE_NOT_FOUND", "pipeline not found or not deleted")
}

func (s *pipelineService) Archive(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Pipeline, error) {
	pipeline, err := s.repo.Archive(ctx, scope, id, updatedBy)
	if err != nil {
		return domain.Pipeline{}, crmmodule.MapNotFound(err, "PIPELINE_NOT_FOUND", "pipeline not found or already deleted")
	}
	return pipeline, nil
}

func (s *pipelineService) ReplaceStages(ctx context.Context, scope coretenant.Scope, id string, stages []repository.StageInput, updatedBy string) (domain.Pipeline, error) {
	pipeline, err := s.repo.ReplaceStages(ctx, scope, id, stages, updatedBy)
	if err != nil {
		return domain.Pipeline{}, crmmodule.MapNotFound(err, "PIPELINE_NOT_FOUND", "pipeline not found or already deleted")
	}
	return pipeline, nil
}
