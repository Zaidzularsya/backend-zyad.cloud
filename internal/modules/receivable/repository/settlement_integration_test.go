//go:build integration

package repository_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

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

	found, err := repo.FindPaymentByReference(e.ctx, ref)
	if err != nil || found.BeneficiaryOrganizationID != orgID || found.InvoiceID != inv.ID || found.Amount != "0.50" {
		t.Fatalf("find = %+v err=%v", found, err)
	}
	if _, err := repo.FindPaymentByReference(e.ctx, ref+"-missing"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing reference: %v", err)
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

func TestSettlementLedgerTotalsAndLockedInsert(t *testing.T) {
	e := setup(t)
	orgA, orgB := e.tenants.A.OrganizationID, e.tenants.B.OrganizationID
	inv := e.issued(t, e.tenants.A.Scope, e.account(t, e.tenants.A.Scope, "Budi").ID, "INV-2026-0961")
	repo := repository.NewSettlementRepository(e.db)
	ledger := repository.NewSettlementLedgerRepository(e.db)
	ref := "it-ledger-" + inv.ID
	t.Cleanup(func() {
		_, _ = e.db.Exec(e.ctx, `DELETE FROM settlement_entries WHERE beneficiary_organization_id IN ($1, $2)`, orgA, orgB)
	})
	if _, err := repo.InsertPayment(e.ctx, repository.SettlementPaymentParams{BeneficiaryOrganizationID: orgA, InvoiceID: inv.ID, ProviderReference: ref, Amount: "150000.50", OccurredAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	okCheck := func(repository.SettlementTotals) error { return nil }
	in := func(kind, amount, note string) repository.SettlementManualEntry {
		return repository.SettlementManualEntry{OrganizationID: orgA, Kind: kind, Amount: amount, Note: note, OccurredAt: time.Now().UTC()}
	}
	if _, err := ledger.InsertManualLocked(e.ctx, in("settlement", "50000.25", ""), okCheck); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.InsertManualLocked(e.ctx, in("adjustment", "-0.25", "koreksi"), okCheck); err != nil {
		t.Fatal(err)
	}
	got, err := ledger.Totals(e.ctx, orgA)
	if err != nil || got != (repository.SettlementTotals{PaidIn: "150000.50", Settled: "50000.25", Adjusted: "-0.25"}) {
		t.Fatalf("totals = %+v err=%v", got, err)
	}
	if other, _ := ledger.Totals(e.ctx, orgB); other != (repository.SettlementTotals{PaidIn: "0.00", Settled: "0.00", Adjusted: "0.00"}) {
		t.Fatalf("org B totals = %+v", other)
	}
	// check yang gagal tidak menulis apa pun
	boom := errors.New("boom")
	if _, err := ledger.InsertManualLocked(e.ctx, in("settlement", "1.00", ""), func(repository.SettlementTotals) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if _, total, _ := ledger.ListEntries(e.ctx, orgA, 50, 0); total != 3 {
		t.Fatalf("entries = %d", total)
	}
	entries, _, _ := ledger.ListEntries(e.ctx, orgA, 50, 0)
	if entries[0].OccurredAt.Before(entries[len(entries)-1].OccurredAt) {
		t.Fatal("entri harus terbaru lebih dulu")
	}
	bals, total, err := ledger.ListBalances(e.ctx, repository.SettlementBalanceFilter{Limit: 200})
	if err != nil || total < 1 || len(bals) < 1 {
		t.Fatalf("balances = %+v total=%d err=%v", bals, total, err)
	}

	// Dua setoran paralel 100% saldo: dengan advisory lock tepat satu yang lolos cek.
	remaining := "100000.00" // 150000.50 - 50000.25 + (-0.25)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ledger.InsertManualLocked(e.ctx, in("settlement", remaining, ""), func(tt repository.SettlementTotals) error {
				if tt.Settled != "50000.25" {
					return errors.New("exceeds")
				}
				return nil
			})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	okCount := 0
	for err := range results {
		if err == nil {
			okCount++
		}
	}
	if okCount != 1 {
		t.Fatalf("setoran paralel lolos %d kali, want 1", okCount)
	}
	if typ, err := ledger.BeneficiaryType(e.ctx, orgA); err != nil || typ != "customer" {
		t.Fatalf("type = %q err=%v", typ, err)
	}
	if _, err := ledger.BeneficiaryType(e.ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, repository.ErrOrganizationNotFound) {
		t.Fatalf("err = %v", err)
	}
}
