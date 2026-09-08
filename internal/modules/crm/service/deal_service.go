package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type dealService struct {
	repo         repository.DealRepository
	pipelineRepo repository.PipelineRepository
}

func NewDealService(repo repository.DealRepository, pipelineRepo repository.PipelineRepository) DealService {
	return &dealService{repo: repo, pipelineRepo: pipelineRepo}
}

func (s *dealService) validateStageInPipeline(ctx context.Context, scope coretenant.Scope, pipelineID string, stageID string) error {
	pipeline, err := s.pipelineRepo.FindByID(ctx, scope, pipelineID)
	if err != nil {
		return crmmodule.MapNotFound(err, "PIPELINE_NOT_FOUND", "pipeline not found or already deleted")
	}
	for _, stage := range pipeline.Stages {
		if stage.ID == stageID {
			return nil
		}
	}
	return ErrDealStageNotInPipeline
}

func (s *dealService) Create(ctx context.Context, scope coretenant.Scope, params repository.CreateDealParams) (domain.Deal, error) {
	if err := s.validateStageInPipeline(ctx, scope, params.PipelineID, params.StageID); err != nil {
		return domain.Deal{}, err
	}
	return s.repo.Create(ctx, scope, params)
}

func (s *dealService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Deal, error) {
	deal, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Deal{}, crmmodule.MapNotFound(err, "DEAL_NOT_FOUND", "deal not found or already deleted")
	}
	return deal, nil
}

func (s *dealService) List(ctx context.Context, scope coretenant.Scope, filter repository.DealListFilter) ([]domain.Deal, int64, error) {
	return s.repo.List(ctx, scope, filter)
}

func (s *dealService) Update(ctx context.Context, scope coretenant.Scope, id string, params repository.UpdateDealParams) (domain.Deal, error) {
	if params.StageID != nil {
		deal, err := s.repo.FindByID(ctx, scope, id)
		if err != nil {
			return domain.Deal{}, crmmodule.MapNotFound(err, "DEAL_NOT_FOUND", "deal not found or already deleted")
		}
		if err := s.validateStageInPipeline(ctx, scope, deal.PipelineID, *params.StageID); err != nil {
			return domain.Deal{}, err
		}
	}
	deal, err := s.repo.Update(ctx, scope, id, params)
	if err != nil {
		return domain.Deal{}, crmmodule.MapNotFound(err, "DEAL_NOT_FOUND", "deal not found or already deleted")
	}
	return deal, nil
}

func (s *dealService) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	return crmmodule.MapNotFound(s.repo.Delete(ctx, scope, id, deletedBy), "DEAL_NOT_FOUND", "deal not found or already deleted")
}

func (s *dealService) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	return crmmodule.MapNotFound(s.repo.Restore(ctx, scope, id, restoredBy), "DEAL_NOT_FOUND", "deal not found or not deleted")
}

func (s *dealService) MoveStage(ctx context.Context, scope coretenant.Scope, id string, stageID string, updatedBy string) (domain.Deal, error) {
	deal, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Deal{}, crmmodule.MapNotFound(err, "DEAL_NOT_FOUND", "deal not found or already deleted")
	}
	if err := s.validateStageInPipeline(ctx, scope, deal.PipelineID, stageID); err != nil {
		return domain.Deal{}, err
	}
	moved, err := s.repo.MoveStage(ctx, scope, id, stageID, updatedBy)
	if err != nil {
		return domain.Deal{}, crmmodule.MapNotFound(err, "DEAL_NOT_OPEN", "deal not found, already deleted, or not open")
	}
	return moved, nil
}

func (s *dealService) CloseWon(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Deal, error) {
	deal, err := s.repo.CloseWon(ctx, scope, id, updatedBy)
	if err != nil {
		return domain.Deal{}, crmmodule.MapNotFound(err, "DEAL_NOT_OPEN", "deal not found, already deleted, or not open")
	}
	return deal, nil
}

func (s *dealService) CloseLost(ctx context.Context, scope coretenant.Scope, id string, lostReason string, updatedBy string) (domain.Deal, error) {
	deal, err := s.repo.CloseLost(ctx, scope, id, lostReason, updatedBy)
	if err != nil {
		return domain.Deal{}, crmmodule.MapNotFound(err, "DEAL_NOT_OPEN", "deal not found, already deleted, or not open")
	}
	return deal, nil
}

func (s *dealService) ApproveDiscount(ctx context.Context, scope coretenant.Scope, id string, discountPercent string, approvedBy string) (domain.Deal, error) {
	deal, err := s.repo.ApproveDiscount(ctx, scope, id, discountPercent, approvedBy)
	if err != nil {
		return domain.Deal{}, crmmodule.MapNotFound(err, "DEAL_NOT_FOUND", "deal not found or already deleted")
	}
	return deal, nil
}
