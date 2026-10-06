//go:build integration

package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database/testutil"
	"zyad.cloud/internal/shared/pricing"
)

// soQuotation membuat quotation dengan 3 baris: sekali·prabayar, sekali·pascabayar, bulanan·pascabayar.
func soQuotation(t *testing.T, repo repository.QuotationRepository, scope coretenant.Scope, number string, withPrepaidOneTime bool) domain.Quotation {
	t.Helper()
	items := []repository.QuotationItemInput{
		{Description: "Website", Quantity: "1.00", UnitPrice: "5000000.00", LineTotal: "5000000.00",
			Pricing: pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Postpaid}},
		{Description: "Maintenance", Quantity: "1.00", UnitPrice: "200000.00", LineTotal: "200000.00", TaxPercent: "11", TaxAmount: "22000.00",
			Pricing: pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Postpaid}},
	}
	if withPrepaidOneTime {
		items = append(items, repository.QuotationItemInput{Description: "Instalasi", Quantity: "1.00", UnitPrice: "500000.00", LineTotal: "500000.00", Position: 2})
	}
	q, err := repo.Create(context.Background(), scope, repository.CreateQuotationParams{
		QuotationNumber: number, Subtotal: "5700000.00", DiscountTotal: "0.00", TaxTotal: "22000.00", GrandTotal: "5722000.00",
		FirstInvoiceTotal: "0.00", RecurringTotals: map[pricing.Frequency]string{pricing.Monthly: "222000.00"}, Items: items,
	})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestSalesOrderLifecycle(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	scope := tenants.A.Scope
	quotations := repository.NewQuotationRepository(db)
	orders := repository.NewSalesOrderRepository(db)
	q := soQuotation(t, quotations, scope, "QUO-T-0401", false)

	so, created, err := orders.CreateFromQuotation(ctx, scope, q, "SO-T-0001", repository.SalesOrderDraftFields{BillToName: "Budi", Channels: []string{"email"}}, "")
	if err != nil || !created {
		t.Fatalf("create: created=%v err=%v", created, err)
	}
	if so.Status != domain.SalesOrderDraft || so.BillingStatus != domain.BillingNone || len(so.Items) != 2 || so.QuotationNumber != "QUO-T-0401" ||
		so.RecurringTotals[pricing.Monthly] != "222000.00" || so.GrandTotal != "5722000.00" || so.BillToName != "Budi" {
		t.Fatalf("so = %+v", so)
	}
	if so.Items[0].DeliveryStatus != domain.DeliveryPending || so.Items[1].DeliveryStatus != domain.DeliveryNotApplicable ||
		so.Items[1].Pricing.Frequency != pricing.Monthly || so.Items[1].TaxAmount != "22000.00" {
		t.Fatalf("items = %+v", so.Items)
	}
	again, created, err := orders.CreateFromQuotation(ctx, scope, q, "SO-T-0002", repository.SalesOrderDraftFields{}, "")
	if err != nil || created || again.ID != so.ID || again.SONumber != "SO-T-0001" {
		t.Fatalf("second create: %+v created=%v err=%v", again, created, err)
	}

	// Konfirmasi paralel: tepat satu menang.
	start := time.Now().UTC()
	if _, err := orders.UpdateDraft(ctx, scope, so.ID, repository.SalesOrderDraftFields{StartDate: &start, BillToName: "Budi", Channels: []string{"email"}}, ""); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = orders.MarkConfirmed(ctx, scope, so.ID, "")
		}()
	}
	wg.Wait()
	wins := 0
	for _, e := range results {
		if e == nil {
			wins++
		} else if !errors.Is(e, repository.ErrSalesOrderNotDraft) {
			t.Fatalf("unexpected err %v", e)
		}
	}
	if wins != 1 {
		t.Fatalf("MarkConfirmed wins = %d, want 1", wins)
	}
	confirmed, _ := orders.FindByID(ctx, scope, so.ID)
	if confirmed.Status != domain.SalesOrderConfirmed || confirmed.BillingStatus != domain.BillingPending || confirmed.ConfirmedAt == nil {
		t.Fatalf("confirmed = %+v", confirmed)
	}
	if _, err := orders.UpdateDraft(ctx, scope, so.ID, repository.SalesOrderDraftFields{BillToName: "X"}, ""); !errors.Is(err, repository.ErrSalesOrderNotDraft) {
		t.Fatalf("UpdateDraft after confirm err = %v", err)
	}
	if _, err := orders.Cancel(ctx, scope, so.ID, ""); !errors.Is(err, repository.ErrSalesOrderNotDraft) {
		t.Fatalf("Cancel after confirm err = %v", err)
	}

	// Billing selesai (tanpa baris prabayar → belum completed karena masih ada yang menunggu diterima).
	done, err := orders.SetBillingResult(ctx, scope, so.ID, domain.BillingDone, "", "", "", "")
	if err != nil || done.BillingStatus != domain.BillingDone || done.Status != domain.SalesOrderConfirmed {
		t.Fatalf("billing done = %+v err=%v", done, err)
	}

	// Konfirmasi diterima.
	if _, err := orders.MarkDelivered(ctx, scope, so.ID, []string{so.Items[1].ID}, start, "", "", ""); !errors.Is(err, repository.ErrDeliveryNotPending) {
		t.Fatalf("deliver not_applicable err = %v", err)
	}
	delivered, err := orders.MarkDelivered(ctx, scope, so.ID, []string{so.Items[0].ID}, start, "Serah terima", "", "")
	if err != nil || delivered.Items[0].DeliveryStatus != domain.DeliveryDelivered || delivered.Items[0].DeliveryNote != "Serah terima" || delivered.Status != domain.SalesOrderCompleted {
		t.Fatalf("delivered = %+v err=%v", delivered, err)
	}
	if _, err := orders.MarkDelivered(ctx, scope, so.ID, []string{so.Items[0].ID}, start, "", "", ""); !errors.Is(err, repository.ErrDeliveryNotPending) {
		t.Fatalf("deliver twice err = %v", err)
	}

	byDeal, _ := orders.ListByDeal(ctx, scope, "11111111-1111-1111-1111-111111111111")
	if len(byDeal) != 0 {
		t.Fatalf("ListByDeal = %d", len(byDeal))
	}
	list, total, err := orders.List(ctx, scope, repository.SalesOrderListFilter{Status: "completed", Search: "so-t-0001"})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("List = %d/%d err=%v", len(list), total, err)
	}

	// Isolasi tenant.
	if _, err := orders.FindByID(ctx, tenants.B.Scope, so.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("tenant B FindByID err = %v", err)
	}
	if _, total, _ := orders.List(ctx, tenants.B.Scope, repository.SalesOrderListFilter{}); total != 0 {
		t.Fatalf("tenant B sees %d orders", total)
	}
}

func TestSalesOrderCompletionWaitsForPrepaidInvoice(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	scope := tenants.A.Scope
	orders := repository.NewSalesOrderRepository(db)
	q := soQuotation(t, repository.NewQuotationRepository(db), scope, "QUO-T-0402", true)
	so, _, err := orders.CreateFromQuotation(ctx, scope, q, "SO-T-0003", repository.SalesOrderDraftFields{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orders.MarkConfirmed(ctx, scope, so.ID, ""); err != nil {
		t.Fatal(err)
	}
	// Terima pascabayar sebelum invoice awal ada: belum completed.
	got, err := orders.MarkDelivered(ctx, scope, so.ID, []string{so.Items[0].ID}, time.Now().UTC(), "", "", "")
	if err != nil || got.Status != domain.SalesOrderConfirmed {
		t.Fatalf("after delivery = %+v err=%v", got, err)
	}
	// Billing selesai dengan invoice awal → completed.
	done, err := orders.SetBillingResult(ctx, scope, so.ID, domain.BillingDone, "", "", "22222222-2222-2222-2222-222222222222", "")
	if err != nil || done.Status != domain.SalesOrderCompleted || done.InitialInvoiceID == "" {
		t.Fatalf("after billing = %+v err=%v", done, err)
	}
}
