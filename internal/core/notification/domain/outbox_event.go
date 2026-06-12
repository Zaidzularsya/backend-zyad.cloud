package domain

import "time"

type OutboxStatus string

const (
	OutboxStatusPending    OutboxStatus = "pending"
	OutboxStatusProcessing OutboxStatus = "processing"
	OutboxStatusSucceeded  OutboxStatus = "succeeded"
	OutboxStatusFailed     OutboxStatus = "failed"
	OutboxStatusDead       OutboxStatus = "dead"
)

type OutboxEvent struct {
	ID             string
	EventType      string
	OrganizationID string
	UserID         string
	Recipient      NotificationRecipient
	Payload        map[string]any
	Locale         string
	Status         OutboxStatus
	Attempts       int
	MaxAttempts    int
	NextRetryAt    *time.Time
	ErrorMessage   string
	ProcessedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
