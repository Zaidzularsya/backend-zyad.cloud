//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestSelfServeQuotationRepository(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	scope := tenants.A.Scope
	pipelineID, stages := createTestPipeline(t, db, scope, "Self-Serve", false)
	deal, err := repository.NewDealRepository(db).Create(ctx, scope, repository.CreateDealParams{PipelineID: pipelineID, StageID: stages[0], Title: "Deal", Value: "1"})
	if err != nil {
		t.Fatal(err)
	}
	quotations := repository.NewQuotationRepository(db)

	if _, err := quotations.FindSelfServeByDeal(ctx, scope, deal.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("none: err = %v", err)
	}
	mk := func(number, channel string) domain.Quotation {
		q, err := quotations.Create(ctx, scope, repository.CreateQuotationParams{
			DealID: deal.ID, QuotationNumber: number, Subtotal: "0", DiscountTotal: "0", TaxTotal: "0", GrandTotal: "0", Channel: channel,
			Items: []repository.QuotationItemInput{{Description: "Paket", Quantity: "1.00", UnitPrice: "100.00", LineTotal: "100.00"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return q
	}
	regular := mk("QUO-REG-1", "")
	if regular.Channel != "" {
		t.Fatalf("regular channel = %q", regular.Channel)
	}
	if _, err := quotations.AcceptSelfServe(ctx, scope, regular.ID, ""); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("regular accept err = %v, want ErrNoRows", err)
	}

	ss := mk("QUO-SS-1", domain.QuotationChannelSelfServe)
	found, err := quotations.FindSelfServeByDeal(ctx, scope, deal.ID)
	if err != nil || found.ID != ss.ID || found.Channel != domain.QuotationChannelSelfServe || len(found.Items) != 1 {
		t.Fatalf("find: %+v err=%v", found, err)
	}
	if other, err := quotations.FindSelfServeByDeal(ctx, tenants.B.Scope, deal.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other org: %+v err=%v", other, err)
	}

	accepted, err := quotations.AcceptSelfServe(ctx, scope, ss.ID, "")
	if err != nil || accepted.Status != domain.QuotationStatusApproved || accepted.ApprovedAt == nil {
		t.Fatalf("accept: %+v err=%v", accepted, err)
	}
	// approved tetap ditemukan; menerima ulang bukan draft → ErrNoRows
	if again, err := quotations.FindSelfServeByDeal(ctx, scope, deal.ID); err != nil || again.Status != domain.QuotationStatusApproved {
		t.Fatalf("find approved: %+v err=%v", again, err)
	}
	if _, err := quotations.AcceptSelfServe(ctx, scope, ss.ID, ""); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second accept err = %v", err)
	}
}

func TestSalesOrderMarkCancelled(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	scope := tenants.A.Scope
	quotations := repository.NewQuotationRepository(db)
	orders := repository.NewSalesOrderRepository(db)

	q := soQuotation(t, quotations, scope, "QUO-T-0451", false)
	so, _, err := orders.CreateFromQuotation(ctx, scope, q, "SO-T-0451", repository.SalesOrderDraftFields{BillToName: "Budi", Channels: []string{"email"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	// draft tidak bisa MarkCancelled
	if _, err := orders.MarkCancelled(ctx, scope, so.ID, ""); !errors.Is(err, repository.ErrSalesOrderNotConfirmed) {
		t.Fatalf("draft err = %v", err)
	}
	if _, err := orders.MarkConfirmed(ctx, scope, so.ID, ""); err != nil {
		t.Fatal(err)
	}
	got, err := orders.MarkCancelled(ctx, scope, so.ID, "")
	if err != nil || got.Status != domain.SalesOrderCancelled || got.CancelledAt == nil {
		t.Fatalf("cancel: %+v err=%v", got, err)
	}
	if _, err := orders.MarkCancelled(ctx, scope, so.ID, ""); !errors.Is(err, repository.ErrSalesOrderNotConfirmed) {
		t.Fatalf("repeat err = %v", err)
	}
	if _, err := orders.MarkCancelled(ctx, tenants.B.Scope, so.ID, ""); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other org err = %v", err)
	}
}
