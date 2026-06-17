package domain

import "time"

type LandingPageRevision struct {
	ID             string
	OrganizationID string
	LandingPageID  string
	RevisionNumber int
	Snapshot       map[string]any
	ChangeNote     string
	CreatedBy      string
	CreatedAt      time.Time
}

type ScheduleAction string

const (
	ScheduleActionPublish   ScheduleAction = "publish"
	ScheduleActionUnpublish ScheduleAction = "unpublish"
)

func (a ScheduleAction) IsValid() bool {
	switch a {
	case ScheduleActionPublish, ScheduleActionUnpublish:
		return true
	default:
		return false
	}
}

type ScheduleStatus string

const (
	ScheduleStatusPending    ScheduleStatus = "pending"
	ScheduleStatusProcessing ScheduleStatus = "processing"
	ScheduleStatusCompleted  ScheduleStatus = "completed"
	ScheduleStatusFailed     ScheduleStatus = "failed"
)

func (s ScheduleStatus) IsValid() bool {
	switch s {
	case ScheduleStatusPending, ScheduleStatusProcessing, ScheduleStatusCompleted, ScheduleStatusFailed:
		return true
	default:
		return false
	}
}

type LandingPageSchedule struct {
	ID             string
	OrganizationID string
	LandingPageID  string
	Action         ScheduleAction
	ScheduledAt    time.Time
	Status         ScheduleStatus
	LockID         *string
	LockExpiresAt  *time.Time
	ErrorMessage   string
	Attempts       int
	CreatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type LandingPageVersion struct {
	ID             string
	OrganizationID string
	LandingPageID  string
	Version        int
	ChangeNote     string
	Snapshot       map[string]any
	CreatedBy      string
	CreatedAt      time.Time
}

type LandingSlugRedirect struct {
	ID             string
	OrganizationID string
	SourceSlug     string
	TargetSlug     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
