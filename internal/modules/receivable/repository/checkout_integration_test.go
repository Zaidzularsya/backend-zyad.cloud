//go:build integration

package repository_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/receivable/repository"
)

func TestCheckoutDirectory(t *testing.T) {
	e := setup(t)
	scopeA, scopeB := e.tenants.A.Scope, e.tenants.B.Scope
	inv := e.issued(t, scopeA, e.account(t, scopeA, "Budi").ID, "INV-2026-0901")
	repo := repository.NewCheckoutRepository(e.db)
	t.Cleanup(func() {
		_, _ = e.db.Exec(e.ctx, `DELETE FROM receivable_checkouts WHERE organization_id = $1`, e.tenants.A.OrganizationID)
	})

	now := time.Now().UTC().Truncate(time.Second)
	number := "RCV-" + inv.ID
	created, err := repo.Create(e.ctx, scopeA, repository.Checkout{
		InvoiceID: inv.ID, DokuInvoiceNumber: number, Amount: "333000", PaymentURL: "https://doku.test/pay", ExpiresAt: now.Add(time.Hour),
	})
	if err != nil || created.Status != "pending" || created.OrganizationID != e.tenants.A.OrganizationID {
		t.Fatalf("created=%+v err=%v", created, err)
	}

	got, err := repo.FindByDokuNumber(e.ctx, number)
	if err != nil || got.OrganizationID != e.tenants.A.OrganizationID || got.InvoiceID != inv.ID {
		t.Fatalf("by number = %+v err=%v", got, err)
	}
	if _, err := repo.FindByDokuNumber(e.ctx, "RCV-unknown"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown number: %v", err)
	}

	if r, err := repo.FindReusable(e.ctx, scopeA, inv.ID, now); err != nil || r.ID != created.ID {
		t.Fatalf("reusable = %+v err=%v", r, err)
	}
	if _, err := repo.FindReusable(e.ctx, scopeA, inv.ID, now.Add(2*time.Hour)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expired session must not be reusable: %v", err)
	}
	if _, err := repo.FindReusable(e.ctx, scopeB, inv.ID, now); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("tenant B must not see A's session: %v", err)
	}

	if err := repo.SetStatus(e.ctx, created.ID, "paid"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindReusable(e.ctx, scopeA, inv.ID, now); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("paid session must not be reusable: %v", err)
	}
}
