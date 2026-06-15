package model

import "time"

type ImpersonationSession struct {
	ID                   string
	OperatorSessionID    string
	OperatorUserID       string
	TargetOrganizationID string
	TargetUserID         string
	Reason               string
	TicketReference      string
	StartedAt            time.Time
	ExpiresAt            time.Time
	StoppedAt            *time.Time
	StoppedByUserID      string
	StopReason           string
	Metadata             map[string]any
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (s ImpersonationSession) IsActive(now time.Time) bool {
	return s.StoppedAt == nil && now.Before(s.ExpiresAt)
}
