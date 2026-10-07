package app

import (
	"context"

	"zyad.cloud/internal/core/middleware"
	organizationservice "zyad.cloud/internal/modules/organization/service"
)

// entitlementChecker adapts *organizationservice.EntitlementGuard
// (which returns the full organization entitlement) to middleware.EntitlementChecker
// (which only needs a pass/fail signal), keeping internal/core free of a
// dependency on internal/modules.
type entitlementChecker struct {
	guard *organizationservice.EntitlementGuard
}

func (a entitlementChecker) RequireFeature(ctx context.Context, organizationID string, featureKey string) error {
	_, err := a.guard.RequireFeature(ctx, organizationID, featureKey)
	return err
}

var _ middleware.EntitlementChecker = entitlementChecker{}
