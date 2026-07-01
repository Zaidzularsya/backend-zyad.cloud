//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/billing/model"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestBillingRepositoryPlanFeatureEntitlementIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	planRepo := NewPlanRepository(db)
	featureRepo := NewFeatureRepository(db)
	entitlementRepo := NewPlanEntitlementRepository(db)
	subscriptionRepo := NewSubscriptionRepository(db)
	invoiceRepo := NewInvoiceRepository(db)
	paymentRepo := NewPaymentRepository(db)
	entitlementSink := NewEntitlementSink(db)
	organizationRepo := organizationrepo.NewOrganizationRepository(db)
	organizationEntitlementRepo := organizationrepo.NewEntitlementRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	plans, total, err := planRepo.List(ctx, PlanListFilter{Limit: 20})
	if err != nil {
		t.Fatalf("List plans error = %v", err)
	}
	if total < 5 || len(plans) < 5 {
		t.Fatalf("List plans total = %d len = %d, want seeded plans", total, len(plans))
	}

	growth, err := planRepo.FindByCode(ctx, "growth")
	if err != nil {
		t.Fatalf("FindByCode(growth) error = %v", err)
	}
	if growth.Code != "growth" || !growth.IsAvailable() {
		t.Fatalf("FindByCode(growth) = %#v", growth)
	}

	landingFeature, err := featureRepo.FindByKey(ctx, "landing.enabled")
	if err != nil {
		t.Fatalf("FindByKey(landing.enabled) error = %v", err)
	}
	if landingFeature.ValueType != model.FeatureValueTypeBoolean {
		t.Fatalf("landing.enabled ValueType = %s", landingFeature.ValueType)
	}

	features, featureTotal, err := featureRepo.List(ctx, FeatureListFilter{
		Module: "landing",
		Limit:  20,
	})
	if err != nil {
		t.Fatalf("List landing features error = %v", err)
	}
	if featureTotal < 6 || len(features) < 6 {
		t.Fatalf("List landing features total = %d len = %d, want seeded landing features", featureTotal, len(features))
	}

	prices, err := planRepo.ListPrices(ctx, growth.ID, false)
	if err != nil {
		t.Fatalf("ListPrices(growth) error = %v", err)
	}
	if len(prices) == 0 {
		t.Fatal("ListPrices(growth) returned no seeded prices")
	}

	seededEntitlements, err := entitlementRepo.ListByPlanID(ctx, growth.ID)
	if err != nil {
		t.Fatalf("ListByPlanID(growth) error = %v", err)
	}
	if len(seededEntitlements) == 0 {
		t.Fatal("ListByPlanID(growth) returned no seeded entitlements")
	}

	testCode := strings.ReplaceAll("repo_"+testutil.UniqueCode("billing"), ".", "_")
	testPlan, err := planRepo.Create(ctx, CreatePlanParams{
		Code:      testCode,
		Name:      "Repository Test Plan",
		Type:      model.PlanTypePaid,
		IsPublic:  false,
		IsActive:  true,
		SortOrder: 999,
	})
	if err != nil {
		t.Fatalf("Create test plan error = %v", err)
	}
	t.Cleanup(func() {
		_ = planRepo.SoftDelete(context.Background(), testPlan.ID)
	})

	price, err := planRepo.UpsertPrice(ctx, UpsertPlanPriceParams{
		PlanID:          testPlan.ID,
		BillingInterval: model.BillingIntervalMonthly,
		Currency:        "IDR",
		Amount:          "12345.67",
		IsActive:        true,
	})
	if err != nil {
		t.Fatalf("UpsertPrice error = %v", err)
	}
	if price.Amount != "12345.67" {
		t.Fatalf("UpsertPrice Amount = %s", price.Amount)
	}

	createdYearlyPrice, err := planRepo.CreatePrice(ctx, CreatePlanPriceParams{
		PlanID:          testPlan.ID,
		BillingInterval: model.BillingIntervalYearly,
		Currency:        "IDR",
		Amount:          "120000.00",
		IsActive:        true,
	})
	if err != nil {
		t.Fatalf("CreatePrice error = %v", err)
	}
	if createdYearlyPrice.BillingInterval != model.BillingIntervalYearly {
		t.Fatalf("CreatePrice interval = %s", createdYearlyPrice.BillingInterval)
	}

	foundPrice, err := planRepo.FindPriceByID(ctx, testPlan.ID, createdYearlyPrice.ID, false)
	if err != nil {
		t.Fatalf("FindPriceByID error = %v", err)
	}
	if foundPrice.ID != createdYearlyPrice.ID {
		t.Fatalf("FindPriceByID ID = %s, want %s", foundPrice.ID, createdYearlyPrice.ID)
	}

	updatedPrice, err := planRepo.UpdatePrice(ctx, UpdatePlanPriceParams{
		ID:       createdYearlyPrice.ID,
		PlanID:   testPlan.ID,
		Amount:   stringPointer("150000.00"),
		IsActive: boolPointer(false),
	})
	if err != nil {
		t.Fatalf("UpdatePrice error = %v", err)
	}
	if updatedPrice.Amount != "150000.00" || updatedPrice.IsActive {
		t.Fatalf("UpdatePrice = %#v", updatedPrice)
	}

	if err := planRepo.SoftDeletePrice(ctx, testPlan.ID, createdYearlyPrice.ID); err != nil {
		t.Fatalf("SoftDeletePrice error = %v", err)
	}
	deletedPrices, err := planRepo.ListPrices(ctx, testPlan.ID, true)
	if err != nil {
		t.Fatalf("ListPrices(includeDeleted) error = %v", err)
	}
	deletedFound := false
	for _, item := range deletedPrices {
		if item.ID == createdYearlyPrice.ID && item.DeletedAt != nil {
			deletedFound = true
			break
		}
	}
	if !deletedFound {
		t.Fatalf("deleted price %s not found in includeDeleted list", createdYearlyPrice.ID)
	}

	enabled := true
	entitlements, err := entitlementRepo.ReplaceByPlanID(ctx, testPlan.ID, []UpsertPlanEntitlementParams{
		{
			FeatureID: landingFeature.ID,
			ValueBool: &enabled,
		},
	})
	if err != nil {
		t.Fatalf("ReplaceByPlanID error = %v", err)
	}
	if len(entitlements) != 1 || entitlements[0].FeatureKey != "landing.enabled" {
		t.Fatalf("ReplaceByPlanID entitlements = %#v", entitlements)
	}
	if entitlements[0].ValueBool == nil || !*entitlements[0].ValueBool {
		t.Fatalf("ReplaceByPlanID ValueBool = %#v", entitlements[0].ValueBool)
	}

	organization, err := organizationRepo.Create(ctx, organizationrepo.CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "billing-repo-" + strings.ReplaceAll(testutil.UniqueCode("org"), ".", "-"),
		Name:          "Billing Repository Organization",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("Create organization error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `
			DELETE FROM billing_payment_events
			WHERE invoice_id IN (
				SELECT id FROM billing_invoices WHERE organization_id = $1::uuid
			)
		`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM billing_payments WHERE organization_id = $1::uuid`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM billing_invoice_items WHERE invoice_id IN (SELECT id FROM billing_invoices WHERE organization_id = $1::uuid)`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM billing_invoices WHERE organization_id = $1::uuid`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM billing_subscription_events WHERE organization_id = $1::uuid`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM billing_subscriptions WHERE organization_id = $1::uuid`, organization.ID)
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
		Reason:         "integration test billing entitlement sync",
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
		"integration test billing entitlement expire",
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

	invoiceNumber := strings.ToUpper(strings.ReplaceAll("INV-"+testutil.UniqueCode("billing"), ".", "-"))
	invoice, err := invoiceRepo.Create(ctx, CreateInvoiceParams{
		OrganizationID: organization.ID,
		SubscriptionID: &subscription.ID,
		InvoiceNumber:  invoiceNumber,
		Status:         model.InvoiceStatusOpen,
		Currency:       "IDR",
		SubtotalAmount: "100000",
		DiscountAmount: "0",
		TaxAmount:      "11000",
		TotalAmount:    "111000",
		DueDate:        &periodEnd,
		Items: []CreateInvoiceItemParams{
			{
				Type:        model.InvoiceItemTypeSubscription,
				Description: "Growth monthly subscription",
				Quantity:    "1",
				UnitAmount:  "100000",
				TotalAmount: "100000",
			},
		},
	})
	if err != nil {
		t.Fatalf("Create invoice error = %v", err)
	}
	if invoice.InvoiceNumber != invoiceNumber || invoice.TotalAmount != "111000.00" {
		t.Fatalf("Create invoice = %#v", invoice)
	}

	items, err := invoiceRepo.ListItems(ctx, invoice.ID)
	if err != nil {
		t.Fatalf("ListItems error = %v", err)
	}
	if len(items) != 1 || items[0].Type != model.InvoiceItemTypeSubscription {
		t.Fatalf("ListItems = %#v", items)
	}

	paidAt := now.Add(time.Hour)
	payment, err := paymentRepo.Create(ctx, CreatePaymentParams{
		InvoiceID:         invoice.ID,
		OrganizationID:    organization.ID,
		Provider:          model.PaymentProviderManual,
		ProviderReference: "manual-" + strings.ReplaceAll(testutil.UniqueCode("payment"), ".", "-"),
		PaymentMethod:     "bank_transfer",
		Status:            model.PaymentStatusPaid,
		Amount:            "111000",
		Currency:          "IDR",
		PaidAt:            &paidAt,
	})
	if err != nil {
		t.Fatalf("Create payment error = %v", err)
	}
	if !payment.IsPaid() || payment.Amount != "111000.00" {
		t.Fatalf("Create payment = %#v", payment)
	}

	updatedInvoice, err := invoiceRepo.UpdateStatus(ctx, UpdateInvoiceStatusParams{
		ID:             invoice.ID,
		OrganizationID: organization.ID,
		Status:         model.InvoiceStatusPaid,
		PaidAt:         &paidAt,
	})
	if err != nil {
		t.Fatalf("Update invoice status error = %v", err)
	}
	if !updatedInvoice.IsPaid() {
		t.Fatalf("Update invoice status = %#v", updatedInvoice)
	}

	paymentEvent, err := paymentRepo.CreateEvent(ctx, PaymentEventParams{
		PaymentID:       &payment.ID,
		InvoiceID:       &invoice.ID,
		Provider:        model.PaymentProviderManual,
		Type:            "manual_payment_recorded",
		ProviderEventID: "manual-event-" + strings.ReplaceAll(testutil.UniqueCode("event"), ".", "-"),
		ProcessedAt:     &paidAt,
	})
	if err != nil {
		t.Fatalf("Create payment event error = %v", err)
	}
	if paymentEvent.PaymentID == nil || *paymentEvent.PaymentID != payment.ID {
		t.Fatalf("Create payment event = %#v", paymentEvent)
	}
}

func stringPointer(value string) *string {
	return &value
}

func boolPointer(value bool) *bool {
	return &value
}
