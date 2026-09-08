package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type activityService struct {
	repo        repository.ActivityRepository
	leadRepo    repository.LeadRepository
	contactRepo repository.ContactRepository
	companyRepo repository.CompanyRepository
	dealRepo    repository.DealRepository
}

func NewActivityService(
	repo repository.ActivityRepository,
	leadRepo repository.LeadRepository,
	contactRepo repository.ContactRepository,
	companyRepo repository.CompanyRepository,
	dealRepo repository.DealRepository,
) ActivityService {
	return &activityService{
		repo:        repo,
		leadRepo:    leadRepo,
		contactRepo: contactRepo,
		companyRepo: companyRepo,
		dealRepo:    dealRepo,
	}
}

// validateRelatedEntity substitutes for the FK constraint crm_activities
// can't have (related_entity_id points at one of four different tables
// depending on related_entity_type — see migrations/000082 doc comment).
func (s *activityService) validateRelatedEntity(ctx context.Context, scope coretenant.Scope, entityType domain.ActivityEntityType, entityID string) error {
	var err error
	switch entityType {
	case domain.ActivityEntityLead:
		_, err = s.leadRepo.FindByID(ctx, scope, entityID)
	case domain.ActivityEntityContact:
		_, err = s.contactRepo.FindByID(ctx, scope, entityID)
	case domain.ActivityEntityCompany:
		_, err = s.companyRepo.FindByID(ctx, scope, entityID)
	case domain.ActivityEntityDeal:
		_, err = s.dealRepo.FindByID(ctx, scope, entityID)
	default:
		return ErrInvalidActivityEntityType
	}
	return crmmodule.MapNotFound(err, "ACTIVITY_RELATED_ENTITY_NOT_FOUND", "related entity not found or already deleted")
}

func (s *activityService) Create(ctx context.Context, scope coretenant.Scope, params repository.CreateActivityParams) (domain.Activity, error) {
	if !params.RelatedEntityType.IsValid() {
		return domain.Activity{}, ErrInvalidActivityEntityType
	}
	if !params.Type.IsValid() {
		return domain.Activity{}, ErrInvalidActivityType
	}
	if err := s.validateRelatedEntity(ctx, scope, params.RelatedEntityType, params.RelatedEntityID); err != nil {
		return domain.Activity{}, err
	}
	return s.repo.Create(ctx, scope, params)
}

func (s *activityService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Activity, error) {
	activity, err := s.repo.FindByID(ctx, scope, id)
	if err != nil {
		return domain.Activity{}, crmmodule.MapNotFound(err, "ACTIVITY_NOT_FOUND", "activity not found or already deleted")
	}
	return activity, nil
}

func (s *activityService) List(ctx context.Context, scope coretenant.Scope, filter repository.ActivityListFilter) ([]domain.Activity, int64, error) {
	return s.repo.List(ctx, scope, filter)
}

func (s *activityService) Update(ctx context.Context, scope coretenant.Scope, id string, params repository.UpdateActivityParams) (domain.Activity, error) {
	if params.Type != nil && !params.Type.IsValid() {
		return domain.Activity{}, ErrInvalidActivityType
	}
	activity, err := s.repo.Update(ctx, scope, id, params)
	if err != nil {
		return domain.Activity{}, crmmodule.MapNotFound(err, "ACTIVITY_NOT_FOUND", "activity not found or already deleted")
	}
	return activity, nil
}

func (s *activityService) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	return crmmodule.MapNotFound(s.repo.Delete(ctx, scope, id, deletedBy), "ACTIVITY_NOT_FOUND", "activity not found or already deleted")
}

func (s *activityService) Complete(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Activity, error) {
	activity, err := s.repo.Complete(ctx, scope, id, updatedBy)
	if err != nil {
		return domain.Activity{}, crmmodule.MapNotFound(err, "ACTIVITY_NOT_PENDING", "activity not found, already deleted, or not pending")
	}
	return activity, nil
}

func (s *activityService) Cancel(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Activity, error) {
	activity, err := s.repo.Cancel(ctx, scope, id, updatedBy)
	if err != nil {
		return domain.Activity{}, crmmodule.MapNotFound(err, "ACTIVITY_NOT_PENDING", "activity not found, already deleted, or not pending")
	}
	return activity, nil
}

func (s *activityService) Assign(ctx context.Context, scope coretenant.Scope, id string, assigneeUserID string, updatedBy string) (domain.Activity, error) {
	activity, err := s.repo.Assign(ctx, scope, id, assigneeUserID, updatedBy)
	if err != nil {
		return domain.Activity{}, crmmodule.MapNotFound(err, "ACTIVITY_NOT_FOUND", "activity not found or already deleted")
	}
	return activity, nil
}
