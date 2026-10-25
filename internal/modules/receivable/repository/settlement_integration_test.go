//go:build integration

package repository_test

import (
	"testing"
	"time"

	"zyad.cloud/internal/modules/receivable/repository"
)

func TestSettlementInsertPaymentIdempotent(t *testing.T) {
	e := setup(t)
	orgID := e.tenants.A.OrganizationID
	inv := e.issued(t, e.tenants.A.Scope, e.account(t, e.tenants.A.Scope, "Budi").ID, "INV-2026-0951")
	repo := repository.NewSettlementRepository(e.db)
	ref := "it-settle-" + inv.ID
	t.Cleanup(func() {
		_, _ = e.db.Exec(e.ctx, `DELETE FROM settlement_entries WHERE provider_reference = $1`, ref)
	})

	p := repository.SettlementPaymentParams{
		BeneficiaryOrganizationID: orgID, InvoiceID: inv.ID, ProviderReference: ref, Amount: "0.50",
		OccurredAt: time.Now().UTC().Truncate(time.Second),
	}
	created, err := repo.InsertPayment(e.ctx, p)
	if err != nil || !created {
		t.Fatalf("first insert created=%v err=%v", created, err)
	}
	p.Amount = "999.00" // notifikasi ulang tidak boleh mengubah atau menggandakan entri
	created, err = repo.InsertPayment(e.ctx, p)
	if err != nil || created {
		t.Fatalf("duplicate insert created=%v err=%v", created, err)
	}

	var count int
	var amount, kind string
	if err := e.db.QueryRow(e.ctx, `SELECT count(*), max(amount::text), max(kind) FROM settlement_entries WHERE provider_reference = $1`, ref).
		Scan(&count, &amount, &kind); err != nil {
		t.Fatal(err)
	}
	if count != 1 || amount != "0.50" || kind != "payment" {
		t.Fatalf("count=%d amount=%s kind=%s", count, amount, kind)
	}

	// CHECK: payment wajib positif dan punya invoice_id.
	if _, err := e.db.Exec(e.ctx, `INSERT INTO settlement_entries (beneficiary_organization_id, kind, amount, invoice_id, provider_reference, occurred_at)
		VALUES ($1, 'payment', 0, $2, $3, now())`, orgID, inv.ID, ref+"-zero"); err == nil {
		t.Fatal("zero payment must violate CHECK")
	}
	if _, err := e.db.Exec(e.ctx, `INSERT INTO settlement_entries (beneficiary_organization_id, kind, amount, provider_reference, occurred_at)
		VALUES ($1, 'payment', 1, $2, now())`, orgID, ref+"-noinv"); err == nil {
		t.Fatal("payment without invoice_id must violate CHECK")
	}
	if _, err := e.db.Exec(e.ctx, `INSERT INTO settlement_entries (beneficiary_organization_id, kind, amount, occurred_at)
		VALUES ($1, 'adjustment', 0, now())`, orgID); err == nil {
		t.Fatal("zero adjustment must violate CHECK")
	}
}
