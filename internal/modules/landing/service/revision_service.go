package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type revisionService struct {
	revisionRepo repository.RevisionRepository
	pageRepo     repository.PageRepository
}

func NewRevisionService(
	revisionRepo repository.RevisionRepository,
	pageRepo repository.PageRepository,
) RevisionService {
	return &revisionService{
		revisionRepo: revisionRepo,
		pageRepo:     pageRepo,
	}
}

func (s *revisionService) AutosaveDraft(ctx context.Context, scope coretenant.Scope, params AutosaveParams) (domain.LandingPageRevision, error) {
	if !scope.IsValid() {
		return domain.LandingPageRevision{}, coretenant.ErrInvalidScope
	}

	// Make sure page exists
	_, err := s.pageRepo.FindByID(ctx, scope, params.PageID)
	if err != nil {
		return domain.LandingPageRevision{}, err
	}

	// Determine next revision number
	var nextRevisionNumber int = 1
	latestRev, err := s.revisionRepo.GetLatestRevision(ctx, scope, params.PageID)
	if err == nil {
		nextRevisionNumber = latestRev.RevisionNumber + 1
	}

	// Create revision
	createParams := repository.CreateRevisionParams{
		LandingPageID:  params.PageID,
		RevisionNumber: nextRevisionNumber,
		Snapshot:       params.Snapshot,
		ChangeNote:     params.ChangeNote,
		CreatedBy:      params.ActorID,
	}
	return s.revisionRepo.CreateRevision(ctx, scope, createParams)
}

func (s *revisionService) ListRevisions(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingPageRevision, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}
	return s.revisionRepo.ListRevisions(ctx, scope, pageID)
}

func (s *revisionService) GetRevision(ctx context.Context, scope coretenant.Scope, revisionID string) (domain.LandingPageRevision, error) {
	if !scope.IsValid() {
		return domain.LandingPageRevision{}, coretenant.ErrInvalidScope
	}
	return s.revisionRepo.GetRevision(ctx, scope, revisionID)
}

func (s *revisionService) RestoreRevision(ctx context.Context, scope coretenant.Scope, revisionID string, actorID string) (domain.LandingPage, error) {
	if !scope.IsValid() {
		return domain.LandingPage{}, coretenant.ErrInvalidScope
	}

	rev, err := s.revisionRepo.GetRevision(ctx, scope, revisionID)
	if err != nil {
		return domain.LandingPage{}, err
	}

	// Update the page to mark it as modified and change status to Draft
	status := domain.PageStatusDraft
	updated, err := s.pageRepo.Update(ctx, scope, rev.LandingPageID, repository.UpdatePageParams{
		Status:    &status,
		UpdatedBy: actorID,
	})

	return updated, err
}

func (s *revisionService) ScheduleAction(ctx context.Context, scope coretenant.Scope, params SchedulePublishParams) (domain.LandingPageSchedule, error) {
	if !scope.IsValid() {
		return domain.LandingPageSchedule{}, coretenant.ErrInvalidScope
	}

	createParams := repository.CreateScheduleParams{
		LandingPageID: params.PageID,
		Action:        params.Action,
		ScheduledAt:   params.ScheduledAt,
		CreatedBy:     params.ActorID,
	}
	return s.revisionRepo.CreateSchedule(ctx, scope, createParams)
}

func (s *revisionService) ListSchedules(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingPageSchedule, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}
	return s.revisionRepo.ListSchedules(ctx, scope, pageID)
}

func (s *revisionService) CancelSchedule(ctx context.Context, scope coretenant.Scope, scheduleID string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}
	return s.revisionRepo.DeleteSchedule(ctx, scope, scheduleID)
}
