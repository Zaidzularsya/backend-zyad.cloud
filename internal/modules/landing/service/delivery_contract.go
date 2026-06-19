package service

import (
	"context"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

// DeliveryService interface defines operations for lead notification and delivery
type DeliveryService interface {
	// Integrations
	CreateIntegration(ctx context.Context, scope tenant.Scope, params repository.CreateIntegrationParams) (domain.LandingLeadIntegration, error)
	FindIntegrationByID(ctx context.Context, scope tenant.Scope, id string) (domain.LandingLeadIntegration, error)
	ListIntegrations(ctx context.Context, scope tenant.Scope) ([]domain.LandingLeadIntegration, error)
	UpdateIntegration(ctx context.Context, scope tenant.Scope, id string, params repository.UpdateIntegrationParams) error
	DeleteIntegration(ctx context.Context, scope tenant.Scope, id string) error

	// Dispatch
	// DispatchFormSubmission creates delivery logs in the outbox for all active integrations
	// matching the form submission criteria.
	DispatchFormSubmission(ctx context.Context, scope tenant.Scope, submissionID string) error

	// Worker Operations
	// ProcessPendingDeliveries fetches pending delivery logs and processes them.
	// Typically called by a background cron/worker.
	ProcessPendingDeliveries(ctx context.Context, scope tenant.Scope, limit int) error
}
