package repository

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type CreateIntegrationParams struct {
	Name         string
	Type         domain.IntegrationType
	Credentials  map[string]any
	EventFilters []any
	IsActive     bool
	CreatedBy    string
}

type UpdateIntegrationParams struct {
	Name         string
	Credentials  map[string]any
	EventFilters []any
	IsActive     bool
	UpdatedBy    string
}

type CreateDeliveryLogParams struct {
	IntegrationID string
	SubmissionID  string
	Status        domain.DeliveryStatus
	NextRetryAt   *time.Time
}

type UpdateDeliveryLogParams struct {
	Status          domain.DeliveryStatus
	ResponsePayload *string
	ErrorMessage    *string
	NextRetryAt     *time.Time
}

type IntegrationRepository interface {
	// Integrations
	CreateIntegration(context.Context, coretenant.Scope, CreateIntegrationParams) (domain.LandingLeadIntegration, error)
	GetIntegration(context.Context, coretenant.Scope, string) (domain.LandingLeadIntegration, error)
	ListIntegrations(context.Context, coretenant.Scope) ([]domain.LandingLeadIntegration, error)
	UpdateIntegration(context.Context, coretenant.Scope, string, UpdateIntegrationParams) error
	DeleteIntegration(context.Context, coretenant.Scope, string) error

	// Delivery Logs
	CreateDeliveryLog(context.Context, coretenant.Scope, CreateDeliveryLogParams) (domain.LandingLeadDeliveryLog, error)
	ListDeliveryLogs(context.Context, coretenant.Scope, string) ([]domain.LandingLeadDeliveryLog, error)

	// Worker Operations
	ClaimPendingDeliveries(context.Context, int) ([]domain.LandingLeadDeliveryLog, error)
	UpdateDeliveryLogStatus(context.Context, string, UpdateDeliveryLogParams) error
}
