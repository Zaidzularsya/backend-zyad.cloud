package service

import (
	"context"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type VisibilityService interface {
	UpdateVisibility(ctx context.Context, scope tenant.Scope, pageID string, visibility domain.PageVisibility, updatedBy string) error
	SetPassword(ctx context.Context, scope tenant.Scope, pageID string, plainPassword string, updatedBy string) error
	RemovePassword(ctx context.Context, scope tenant.Scope, pageID string, updatedBy string) error
	VerifyAccess(ctx context.Context, organizationID string, pageID string, submittedPassword string, ipAddress string) (string, error)
	ValidateAccessGrant(ctx context.Context, pageID string, token string) error
}
