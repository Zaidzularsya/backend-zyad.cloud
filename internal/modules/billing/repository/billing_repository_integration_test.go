//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/billing/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	productrepo "zyad.cloud/internal/modules/product/repository"
	subscriptionmodel "zyad.cloud/internal/modules/subscription/model"
	subscriptionrepo "zyad.cloud/internal/modules/subscription/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestBillingRepositoryInvoicePaymentIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	planRepo := productrepo.NewPlanRepository(db)
	subRepo := subscriptionrepo.NewSubscriptionRepository(db)
	invoiceRepo := NewInvoiceRepository(db)
	paymentRepo := NewPaymentRepository(db)
	organizationRepo := organizationrepo.NewOrganizationRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	growth, err := planRepo.FindByCode(ctx, "growth")
	if err != nil {
		t.Fatalf("FindByCode(growth) error = %v", err)
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
		_, _ = db.Exec(cleanupCtx, `DELETE FROM subscription_events WHERE organization_id = $1::uuid`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM customer_subscriptions WHERE organization_id = $1::uuid`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organization_entitlements WHERE organization_id = $1::uuid`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE organization_id = $1::uuid`, organization.ID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1::uuid`, organization.ID)
	})

	now := time.Now().UTC().Truncate(time.Second)
	periodEnd := now.AddDate(0, 1, 0)
	subscription, err := subRepo.Create(ctx, subscriptionrepo.CreateSubscriptionParams{
		OrganizationID:     organization.ID,
		PlanID:             growth.ID,
		Status:             subscriptionmodel.SubscriptionStatusActive,
		BillingInterval:    subscriptionmodel.BillingIntervalMonthly,
		CurrentPeriodStart: &now,
		CurrentPeriodEnd:   &periodEnd,
	})
	if err != nil {
		t.Fatalf("Create subscription error = %v", err)
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
