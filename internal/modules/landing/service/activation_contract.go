package service

import (
	"context"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

// ActivationPreviewURLTemplate is the public preview path; {token} is the raw
// token, which is only returned once when the activation is created.
const ActivationPreviewURLTemplate = "/api/v1/public/landing/preview/{token}"

// ActivationReader exposes page activation state to the admin page API.
type ActivationReader interface {
	// OpenActivationsByPages returns the open activation of each page, keyed by page ID.
	OpenActivationsByPages(ctx context.Context, scope tenant.Scope, pageIDs []string) (map[string]domain.LandingPageActivation, error)
}
