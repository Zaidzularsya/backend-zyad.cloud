//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	productrepo "zyad.cloud/internal/modules/product/repository"
	"zyad.cloud/internal/modules/subscription/model"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestSubscriptionRepositoryIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	planRepo := productrepo.NewPlanRepository(db)
	entitlementRepo := productrepo.NewPlanEntitlementRepository(db)
	subscriptionRepo := NewSubscriptionRepository(db)
	entitlementSink := NewEntitlementSink(db)
	organizationRepo := organizationrepo.NewOrganizationRepository(db)
	organizationEntitlementRepo := organizationrepo.NewEntitlementRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	growth, err := planRepo.FindByCode(ctx, "growth")
	if err != nil {
		t.Fatalf("FindByCode(growth) error = %v", err)
	}

	seededEntitlements, err := entitlementRepo.ListByPlanID(ctx, growth.ID)
	if err != nil {
		t.Fatalf("ListByPlanID(growth) error = %v", err)
	}
	if len(seededEntitlements) == 0 {
		t.Fatal("ListByPlanID(growth) returned no seeded entitlements")
	}

	organization, err := organizationRepo.Create(ctx, organizationrepo.CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "subscription-repo-" + strings.ReplaceAll(testutil.UniqueCode("org"), ".", "-"),
		Name:          "Subscription Repository Organization",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("Create organization error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM subscription_events WHERE organization_id = $1::uuid`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM customer_subscriptions WHERE organization_id = $1::uuid`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organization_entitlements WHERE organization_id = $1::uuid`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE organization_id = $1::uuid`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1::uuid`, organization.ID)
	})

	now := time.Now().UTC().Truncate(time.Second)
	periodEnd := now.AddDate(0, 1, 0)
	subscription, err := subscriptionRepo.Create(ctx, CreateSubscriptionParams{
		OrganizationID:     organization.ID,
		PlanID:             growth.ID,
		Status:             model.SubscriptionStatusActive,
		BillingInterval:    model.BillingIntervalMonthly,
		CurrentPeriodStart: &now,
		CurrentPeriodEnd:   &periodEnd,
	})
	if err != nil {
		t.Fatalf("Create subscription error = %v", err)
	}
	if !subscription.IsUsable() || subscription.OrganizationID != organization.ID {
		t.Fatalf("Create subscription = %#v", subscription)
	}

	usable, err := subscriptionRepo.FindUsableByOrganization(ctx, organization.ID)
	if err != nil {
		t.Fatalf("FindUsableByOrganization error = %v", err)
	}
	if usable.ID != subscription.ID {
		t.Fatalf("FindUsableByOrganization ID = %s, want %s", usable.ID, subscription.ID)
	}

	syncedEntitlements, err := entitlementSink.SyncPlanEntitlements(ctx, SyncPlanEntitlementsParams{
		OrganizationID: organization.ID,
		SubscriptionID: subscription.ID,
		Entitlements:   seededEntitlements,
		EffectiveFrom:  now,
		EffectiveUntil: &periodEnd,
		Reason:         "integration test subscription entitlement sync",
	})
	if err != nil {
		t.Fatalf("SyncPlanEntitlements error = %v", err)
	}
	if len(syncedEntitlements) != len(seededEntitlements) {
		t.Fatalf("SyncPlanEntitlements len = %d, want %d", len(syncedEntitlements), len(seededEntitlements))
	}

	effectiveLandingLimit, err := organizationEntitlementRepo.FindEffective(
		ctx,
		organization.ID,
		"landing.max_pages",
		now.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("FindEffective(landing.max_pages) error = %v", err)
	}
	if effectiveLandingLimit.Source != organizationmodel.EntitlementSourcePlan {
		t.Fatalf("FindEffective source = %s, want plan", effectiveLandingLimit.Source)
	}
	if effectiveLandingLimit.SourceReference != subscription.ID {
		t.Fatalf("FindEffective source_reference = %s, want %s", effectiveLandingLimit.SourceReference, subscription.ID)
	}
	if effectiveLandingLimit.Limits["limit"] == nil || effectiveLandingLimit.Limits["value"] == nil {
		t.Fatalf("FindEffective landing.max_pages limits = %#v", effectiveLandingLimit.Limits)
	}

	expiredCount, err := entitlementSink.ExpirePlanEntitlements(
		ctx,
		organization.ID,
		subscription.ID,
		periodEnd,
		"",
		"integration test subscription entitlement expire",
	)
	if err != nil {
		t.Fatalf("ExpirePlanEntitlements error = %v", err)
	}
	if expiredCount != int64(len(seededEntitlements)) {
		t.Fatalf("ExpirePlanEntitlements count = %d, want %d", expiredCount, len(seededEntitlements))
	}

	newStatus := model.SubscriptionStatusPastDue
	event, err := subscriptionRepo.CreateEvent(ctx, SubscriptionEventParams{
		SubscriptionID: subscription.ID,
		OrganizationID: organization.ID,
		Type:           "subscription_created",
		NewStatus:      &newStatus,
	})
	if err != nil {
		t.Fatalf("Create subscription event error = %v", err)
	}
	if event.SubscriptionID != subscription.ID || event.Type != "subscription_created" {
		t.Fatalf("Create subscription event = %#v", event)
	}
}
