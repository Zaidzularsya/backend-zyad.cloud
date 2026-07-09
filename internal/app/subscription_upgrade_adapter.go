package app

import (
	"context"
	"time"

	billingservice "zyad.cloud/internal/modules/billing/service"
	subscriptionservice "zyad.cloud/internal/modules/subscription/service"
)

// subscriptionUpgradeActivatorAdapter adapts the subscription module's
// SubscriptionService to billing's PaymentSubscriptionUpgradeActivator
// interface. The two modules intentionally do not import each other's
// service packages (subscription is a lower-level domain that billing's
// composition layer depends on, not vice versa), so this adapter lives in
// the wiring layer where both are already in scope.
type subscriptionUpgradeActivatorAdapter struct {
	subscriptions *subscriptionservice.SubscriptionService
}

func (a subscriptionUpgradeActivatorAdapter) ActivateUpgradeByInvoice(
	ctx context.Context,
	invoice billingservice.SubscriptionUpgradeInvoice,
	paidAt time.Time,
) error {
	return a.subscriptions.ActivateUpgradeByInvoice(ctx, subscriptionservice.SubscriptionInvoice{
		OrganizationID: invoice.OrganizationID,
		SubscriptionID: invoice.SubscriptionID,
		Metadata:       invoice.Metadata,
	}, paidAt)
}
