package domain

import "time"

type IntegrationType string

const (
	IntegrationTypeNotification IntegrationType = "notification"
	IntegrationTypeWebhook      IntegrationType = "webhook"
	IntegrationTypeCRM          IntegrationType = "crm"
	IntegrationTypeTelegram     IntegrationType = "telegram"
	IntegrationTypeGoogleSheets IntegrationType = "google_sheets"
)

func (t IntegrationType) IsValid() bool {
	switch t {
	case IntegrationTypeNotification, IntegrationTypeWebhook, IntegrationTypeCRM, IntegrationTypeTelegram, IntegrationTypeGoogleSheets:
		return true
	default:
		return false
	}
}

type LandingLeadIntegration struct {
	ID             string
	OrganizationID string
	Name           string
	Type           IntegrationType
	Credentials    map[string]any
	EventFilters   []any
	IsActive       bool
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

type DeliveryStatus string

const (
	DeliveryStatusPending DeliveryStatus = "pending"
	DeliveryStatusSuccess DeliveryStatus = "success"
	DeliveryStatusFailed  DeliveryStatus = "failed"
)

func (s DeliveryStatus) IsValid() bool {
	switch s {
	case DeliveryStatusPending, DeliveryStatusSuccess, DeliveryStatusFailed:
		return true
	default:
		return false
	}
}

type LandingLeadDeliveryLog struct {
	ID              string
	OrganizationID  string
	IntegrationID   string
	SubmissionID    string
	Status          DeliveryStatus
	ResponsePayload *string
	ErrorMessage    *string
	Attempts        int
	NextRetryAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
