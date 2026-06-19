package service

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type AutosaveParams struct {
	PageID     string
	Snapshot   map[string]any
	ChangeNote string
	ActorID    string
}

type SchedulePublishParams struct {
	PageID      string
	Action      domain.ScheduleAction
	ScheduledAt time.Time
	ActorID     string
}

type RevisionService interface {
	// Revisions
	AutosaveDraft(ctx context.Context, scope coretenant.Scope, params AutosaveParams) (domain.LandingPageRevision, error)
	ListRevisions(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingPageRevision, error)
	GetRevision(ctx context.Context, scope coretenant.Scope, revisionID string) (domain.LandingPageRevision, error)
	RestoreRevision(ctx context.Context, scope coretenant.Scope, revisionID string, actorID string) (domain.LandingPage, error)

	// Schedules
	ScheduleAction(ctx context.Context, scope coretenant.Scope, params SchedulePublishParams) (domain.LandingPageSchedule, error)
	ListSchedules(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingPageSchedule, error)
	CancelSchedule(ctx context.Context, scope coretenant.Scope, scheduleID string) error
}
