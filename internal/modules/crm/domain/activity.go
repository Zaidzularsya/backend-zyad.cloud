package domain

import "time"

type ActivityEntityType string

const (
	ActivityEntityLead    ActivityEntityType = "lead"
	ActivityEntityContact ActivityEntityType = "contact"
	ActivityEntityCompany ActivityEntityType = "company"
	ActivityEntityDeal    ActivityEntityType = "deal"
)

func (t ActivityEntityType) IsValid() bool {
	switch t {
	case ActivityEntityLead, ActivityEntityContact, ActivityEntityCompany, ActivityEntityDeal:
		return true
	default:
		return false
	}
}

type ActivityType string

const (
	ActivityTypeCall    ActivityType = "call"
	ActivityTypeEmail   ActivityType = "email"
	ActivityTypeMeeting ActivityType = "meeting"
	ActivityTypeTask    ActivityType = "task"
	ActivityTypeNote    ActivityType = "note"
)

func (t ActivityType) IsValid() bool {
	switch t {
	case ActivityTypeCall, ActivityTypeEmail, ActivityTypeMeeting, ActivityTypeTask, ActivityTypeNote:
		return true
	default:
		return false
	}
}

type ActivityStatus string

const (
	ActivityStatusPending   ActivityStatus = "pending"
	ActivityStatusCompleted ActivityStatus = "completed"
	ActivityStatusCancelled ActivityStatus = "cancelled"
)

func (s ActivityStatus) IsValid() bool {
	switch s {
	case ActivityStatusPending, ActivityStatusCompleted, ActivityStatusCancelled:
		return true
	default:
		return false
	}
}

// Activity is a polymorphic interaction log entry: RelatedEntityID points at
// one of four different tables depending on RelatedEntityType (no FK — see
// migrations/000082_create_crm_activities.up.sql for why, and
// ActivityService.validateRelatedEntity for the service-layer existence
// check that substitutes for it).
type Activity struct {
	ID                string
	OrganizationID    string
	RelatedEntityType ActivityEntityType
	RelatedEntityID   string
	Type              ActivityType
	Subject           string
	Description       string
	DueAt             *time.Time
	CompletedAt       *time.Time
	Status            ActivityStatus
	AssigneeUserID    string
	CreatedBy         string
	UpdatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
}
