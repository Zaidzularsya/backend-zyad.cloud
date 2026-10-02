//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestQuotationItemSnapshotColumnsRoundTrip(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)

	q, err := repository.NewQuotationRepository(db).Create(ctx, tenants.A.Scope, repository.CreateQuotationParams{
		QuotationNumber: "QUO-T-0001", Subtotal: "100.00", DiscountTotal: "0.00", TaxTotal: "11.00", GrandTotal: "111.00",
		Items: []repository.QuotationItemInput{{
			Description: "Internet 50 Mbps", Quantity: "1.00", UnitPrice: "100.00", LineTotal: "100.00",
			SKU: "NET-50", Unit: "bulan", TaxPercent: "11", TaxAmount: "11.00",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	it := q.Items[0]
	if it.SKU != "NET-50" || it.Unit != "bulan" || it.TaxPercent != "11.00" || it.TaxAmount != "11.00" || it.ProductID != nil {
		t.Fatalf("item = %+v", it)
	}
	if q.RevisionNo != 0 || q.RevisionOfID != nil || q.PDFAssetID != nil {
		t.Fatalf("quotation = %+v", q)
	}
}

func newDraft(t *testing.T, repo repository.QuotationRepository, scope coretenant.Scope, number string) domain.Quotation {
	t.Helper()
	q, err := repo.Create(context.Background(), scope, repository.CreateQuotationParams{
		QuotationNumber: number, Subtotal: "100.00", DiscountTotal: "0.00", TaxTotal: "0.00", GrandTotal: "100.00",
		Items: []repository.QuotationItemInput{{Description: "A", Quantity: "1.00", UnitPrice: "100.00", LineTotal: "100.00"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestQuotationDraftLockAndReplaceItems(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	repo := repository.NewQuotationRepository(db)
	q := newDraft(t, repo, tenants.A.Scope, "QUO-T-0101")

	got, err := repo.ReplaceItems(ctx, tenants.A.Scope, q.ID, repository.ReplaceQuotationItemsParams{
		Items: []repository.QuotationItemInput{
			{Description: "B", Quantity: "2.00", UnitPrice: "50.00", LineTotal: "100.00", TaxPercent: "11", TaxAmount: "11.00"},
			{Description: "C", Quantity: "1.00", UnitPrice: "10.00", LineTotal: "10.00", Position: 1},
		},
		Subtotal: "110.00", DiscountTotal: "0.00", TaxTotal: "11.00", GrandTotal: "121.00",
	})
	if err != nil || len(got.Items) != 2 || got.GrandTotal != "121.00" || got.Items[0].Description != "B" {
		t.Fatalf("replace = %+v err=%v", got, err)
	}

	if _, err := repo.Send(ctx, tenants.A.Scope, q.ID, ""); err != nil {
		t.Fatal(err)
	}
	notes := "x"
	if _, err := repo.Update(ctx, tenants.A.Scope, q.ID, repository.UpdateQuotationParams{Notes: &notes}); !errors.Is(err, repository.ErrQuotationLocked) {
		t.Fatalf("update sent err = %v", err)
	}
	if _, err := repo.ReplaceItems(ctx, tenants.A.Scope, q.ID, repository.ReplaceQuotationItemsParams{
		Items:    []repository.QuotationItemInput{{Description: "Z", Quantity: "1.00", UnitPrice: "1.00", LineTotal: "1.00"}},
		Subtotal: "1.00", DiscountTotal: "0.00", TaxTotal: "0.00", GrandTotal: "1.00",
	}); !errors.Is(err, repository.ErrQuotationLocked) {
		t.Fatalf("replace sent err = %v", err)
	}
	if err := repo.Delete(ctx, tenants.A.Scope, q.ID, ""); !errors.Is(err, repository.ErrQuotationLocked) {
		t.Fatalf("delete sent err = %v", err)
	}
}

func TestQuotationReviseChain(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	repo := repository.NewQuotationRepository(db)
	q := newDraft(t, repo, tenants.A.Scope, "QUO-T-0007")

	params := func(number string, no int) repository.CreateQuotationParams {
		return repository.CreateQuotationParams{
			QuotationNumber: number, RevisionNo: no, Subtotal: "100.00", DiscountTotal: "0.00", TaxTotal: "0.00", GrandTotal: "100.00",
			Items: []repository.QuotationItemInput{{Description: "A", Quantity: "1.00", UnitPrice: "100.00", LineTotal: "100.00"}},
		}
	}
	if _, err := repo.Revise(ctx, tenants.A.Scope, q.ID, params("QUO-T-0007-R1", 1)); !errors.Is(err, repository.ErrQuotationNotRevisable) {
		t.Fatalf("revise draft err = %v", err)
	}
	if _, err := repo.Send(ctx, tenants.A.Scope, q.ID, ""); err != nil {
		t.Fatal(err)
	}
	r1, err := repo.Revise(ctx, tenants.A.Scope, q.ID, params("QUO-T-0007-R1", 1))
	if err != nil || r1.Status != "draft" || r1.RevisionOfID == nil || *r1.RevisionOfID != q.ID || r1.RevisionNo != 1 {
		t.Fatalf("r1 = %+v err=%v", r1, err)
	}
	old, _ := repo.FindByID(ctx, tenants.A.Scope, q.ID)
	if old.Status != "superseded" {
		t.Fatalf("old status = %s", old.Status)
	}
	if _, err := repo.Send(ctx, tenants.A.Scope, r1.ID, ""); err != nil {
		t.Fatal(err)
	}
	r2, err := repo.Revise(ctx, tenants.A.Scope, r1.ID, params("QUO-T-0007-R2", 2))
	if err != nil || r2.QuotationNumber != "QUO-T-0007-R2" {
		t.Fatalf("r2 = %+v err=%v", r2, err)
	}
	// Revisi dari versi yang sudah superseded ditolak.
	if _, err := repo.Revise(ctx, tenants.A.Scope, q.ID, params("QUO-T-0007-R9", 9)); !errors.Is(err, repository.ErrQuotationNotRevisable) {
		t.Fatalf("revise superseded err = %v", err)
	}
}

func TestQuotationExpireDueAndPDFSnapshot(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	repo := repository.NewQuotationRepository(db)

	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	q, _ := repo.Create(ctx, tenants.A.Scope, repository.CreateQuotationParams{
		QuotationNumber: "QUO-T-0201", ValidUntil: &past, Subtotal: "1.00", DiscountTotal: "0.00", TaxTotal: "0.00", GrandTotal: "1.00",
		Items: []repository.QuotationItemInput{{Description: "A", Quantity: "1.00", UnitPrice: "1.00", LineTotal: "1.00"}},
	})
	_, _ = repo.Send(ctx, tenants.A.Scope, q.ID, "")
	n, err := repo.ExpireDue(ctx, tenants.A.Scope, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if err != nil || n != 1 {
		t.Fatalf("expired = %d err=%v", n, err)
	}
	if n, _ := repo.ExpireDue(ctx, tenants.B.Scope, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)); n != 0 {
		t.Fatal("tenant B must not expire tenant A quotations")
	}

	// pdf_asset_id tidak ber-FK (lihat migration Task 1), UUID tetap cukup.
	assetID := "aaaaaaaa-0000-0000-0000-000000000001"
	ok, err := repo.SetPDFSnapshot(ctx, tenants.A.Scope, q.ID, assetID)
	if err != nil || !ok {
		t.Fatalf("first snapshot ok=%v err=%v", ok, err)
	}
	if ok, _ := repo.SetPDFSnapshot(ctx, tenants.A.Scope, q.ID, assetID); ok {
		t.Fatal("second snapshot must be a no-op")
	}
	got, _ := repo.FindByID(ctx, tenants.A.Scope, q.ID)
	if got.PDFAssetID == nil || got.PDFGeneratedAt == nil {
		t.Fatalf("snapshot not stored: %+v", got)
	}
}
