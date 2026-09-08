package app

import (
	"context"

	"zyad.cloud/internal/core/middleware"
	subscriptionservice "zyad.cloud/internal/modules/subscription/service"
)

// subscriptionEntitlementChecker adapts *subscriptionservice.SubscriptionGuardService
// (which returns the full organization entitlement) to middleware.EntitlementChecker
// (which only needs a pass/fail signal), keeping internal/core free of a
// dependency on internal/modules.
type subscriptionEntitlementChecker struct {
	guard *subscriptionservice.SubscriptionGuardService
}

func (a subscriptionEntitlementChecker) RequireFeature(ctx context.Context, organizationID string, featureKey string) error {
	_, err := a.guard.RequireFeature(ctx, organizationID, featureKey)
	return err
}

var _ middleware.EntitlementChecker = subscriptionEntitlementChecker{}
