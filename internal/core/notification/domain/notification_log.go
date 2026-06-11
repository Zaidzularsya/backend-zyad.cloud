package domain

import "time"

type LogStatus string

const (
	LogStatusPending    LogStatus = "pending"
	LogStatusProcessing LogStatus = "processing"
	LogStatusSent       LogStatus = "sent"
	LogStatusFailed     LogStatus = "failed"
	LogStatusCancelled  LogStatus = "cancelled"
	LogStatusDead       LogStatus = "dead"
)

type NotificationLog struct {
	ID                     string
	EventID                string
	EventType              string
	TemplateID             string
	TemplateCode           string
	TemplateVersion        int
	OrganizationID         string
	Channel                Channel
	RecipientType          string
	RecipientUserID        string
	RecipientNameSnapshot  string
	RecipientEmailSnapshot string
	RecipientPhoneSnapshot string
	Destination            string
	Subject                string
	Body                   string
	Status                 LogStatus
	Provider               string
	ProviderMessageID      string
	ProviderResponse       map[string]any
	Attempts               int
	MaxAttempts            int
	NextRetryAt            *time.Time
	ErrorMessage           string
	SentAt                 *time.Time
	FailedAt               *time.Time
	CancelledAt            *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (s LogStatus) IsValid() bool {
	switch s {
	case LogStatusPending,
		LogStatusProcessing,
		LogStatusSent,
		LogStatusFailed,
		LogStatusCancelled,
		LogStatusDead:
		return true
	default:
		return false
	}
}

func (l NotificationLog) CanRetry(now time.Time) bool {
	if l.Status != LogStatusPending && l.Status != LogStatusFailed {
		return false
	}
	if l.Attempts >= l.MaxAttempts {
		return false
	}
	return l.NextRetryAt == nil || !l.NextRetryAt.After(now)
}

func (l NotificationLog) IsFinished() bool {
	switch l.Status {
	case LogStatusSent,
		LogStatusCancelled,
		LogStatusDead:
		return true
	default:
		return false
	}
}
