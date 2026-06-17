package repository

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type CreateRevisionParams struct {
	LandingPageID  string
	RevisionNumber int
	Snapshot       map[string]any
	ChangeNote     string
	CreatedBy      string
}

type CreateScheduleParams struct {
	LandingPageID string
	Action        domain.ScheduleAction
	ScheduledAt   time.Time
	CreatedBy     string
}

type RevisionRepository interface {
	// Revisions
	CreateRevision(context.Context, coretenant.Scope, CreateRevisionParams) (domain.LandingPageRevision, error)
	GetRevision(context.Context, coretenant.Scope, string) (domain.LandingPageRevision, error)
	ListRevisions(context.Context, coretenant.Scope, string) ([]domain.LandingPageRevision, error)
	GetLatestRevision(context.Context, coretenant.Scope, string) (domain.LandingPageRevision, error)

	// Schedules
	CreateSchedule(context.Context, coretenant.Scope, CreateScheduleParams) (domain.LandingPageSchedule, error)
	GetSchedule(context.Context, coretenant.Scope, string) (domain.LandingPageSchedule, error)
	ListSchedules(context.Context, coretenant.Scope, string) ([]domain.LandingPageSchedule, error)
	DeleteSchedule(context.Context, coretenant.Scope, string) error

	// Worker Operations
	ClaimPendingSchedules(context.Context, int, time.Duration) ([]domain.LandingPageSchedule, error)
	MarkScheduleStatus(context.Context, string, domain.ScheduleStatus, *string) error
}
