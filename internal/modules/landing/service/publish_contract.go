package service

import (
	"context"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type PublishChecklist struct {
	Errors   []string
	Warnings []string
	IsValid  bool
}

type PublishService interface {
	// Publish Validation
	ValidateForPublish(ctx context.Context, scope tenant.Scope, pageID string) (PublishChecklist, error)

	// Publish Operations
	Publish(ctx context.Context, scope tenant.Scope, pageID string, changeNote string, publishedBy string) (domain.LandingPageVersion, error)
	Unpublish(ctx context.Context, scope tenant.Scope, pageID string, updatedBy string) error
	RestoreVersion(ctx context.Context, scope tenant.Scope, pageID string, versionID string, restoredBy string) (domain.LandingPage, error)

	// Preview
	GeneratePreviewToken(ctx context.Context, scope tenant.Scope, pageID string, durationMinutes int) (string, error)
	ValidatePreviewToken(ctx context.Context, token string) (string, error) // returns PageID
}
