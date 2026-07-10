package app

import (
	"context"
	"errors"
	"fmt"

	productservice "zyad.cloud/internal/modules/product/service"
	subscriptiondto "zyad.cloud/internal/modules/subscription/dto"
	subscriptionmodel "zyad.cloud/internal/modules/subscription/model"
	subscriptionservice "zyad.cloud/internal/modules/subscription/service"
)

const defaultSubscriptionPlanCode = "free"

// defaultSubscriptionProvisioner creates the default (free) subscription for
// an organization that does not have one yet. It composes the product and
// subscription services from the wiring layer so organization/billing modules
// only depend on a minimal provisioner interface instead of each other.
type defaultSubscriptionProvisioner struct {
	plans         *productservice.PlanService
	subscriptions *subscriptionservice.SubscriptionService
}

func (p defaultSubscriptionProvisioner) ProvisionDefaultSubscription(
	ctx context.Context,
	organizationID string,
) error {
	if p.plans == nil || p.subscriptions == nil {
		return errors.New("default subscription provisioner dependencies are incomplete")
	}
	if _, err := p.subscriptions.FindLatestByOrganization(ctx, organizationID); err == nil {
		return nil
	}
	plan, err := p.plans.FindByCode(ctx, defaultSubscriptionPlanCode, false)
	if err != nil {
		return fmt.Errorf("find default plan %q: %w", defaultSubscriptionPlanCode, err)
	}
	_, err = p.subscriptions.Create(ctx, subscriptiondto.CreateSubscriptionRequest{
		OrganizationID:  organizationID,
		PlanID:          plan.ID,
		BillingInterval: string(subscriptionmodel.BillingIntervalMonthly),
		Status:          string(subscriptionmodel.SubscriptionStatusActive),
		Metadata:        map[string]any{"source": "self_serve_onboarding"},
	})
	if err != nil {
		return fmt.Errorf("create default subscription: %w", err)
	}
	return nil
}
