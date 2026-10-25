package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"zyad.cloud/internal/modules/receivable/repository"
)

type fakeSettlementRepo struct {
	rows []repository.SettlementPaymentParams
	err  error
}

func (f *fakeSettlementRepo) InsertPayment(_ context.Context, p repository.SettlementPaymentParams) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	for _, r := range f.rows {
		if r.ProviderReference == p.ProviderReference {
			return false, nil
		}
	}
	f.rows = append(f.rows, p)
	return true, nil
}

const ledgerOrg = "11111111-1111-1111-1111-111111111111"

func TestSettlementRecorderKeepsDecimalString(t *testing.T) {
	for _, amount := range []string{"150000.00", "0.50", "833000"} {
		repo := &fakeSettlementRepo{}
		rec := NewSettlementRecorder(repo)
		paidAt := time.Date(2026, 10, 25, 10, 0, 0, 0, time.FixedZone("WIB", 7*3600))
		if err := rec.RecordPayment(context.Background(), ledgerOrg, "inv-1", "req-1", amount, paidAt); err != nil {
			t.Fatalf("amount %s: %v", amount, err)
		}
		got := repo.rows[0]
		if got.Amount != amount || got.BeneficiaryOrganizationID != ledgerOrg || got.InvoiceID != "inv-1" || got.ProviderReference != "req-1" {
			t.Fatalf("row = %+v", got)
		}
		if got.OccurredAt.Location() != time.UTC || !got.OccurredAt.Equal(paidAt) {
			t.Fatalf("occurred_at = %v, want UTC of %v", got.OccurredAt, paidAt)
		}
	}
}

func TestSettlementRecorderRejectsInvalidInput(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name, org, inv, ref, amount string
		paidAt                      time.Time
	}{
		{"zero amount", ledgerOrg, "inv-1", "r", "0", now},
		{"negative", ledgerOrg, "inv-1", "r", "-1", now},
		{"three decimals", ledgerOrg, "inv-1", "r", "1.234", now},
		{"not a number", ledgerOrg, "inv-1", "r", "abc", now},
		{"float notation", ledgerOrg, "inv-1", "r", "1e5", now},
		{"empty org", "", "inv-1", "r", "1", now},
		{"empty invoice", ledgerOrg, "", "r", "1", now},
		{"blank reference", ledgerOrg, "inv-1", "  ", "1", now},
		{"zero paid at", ledgerOrg, "inv-1", "r", "1", time.Time{}},
	}
	for _, c := range cases {
		repo := &fakeSettlementRepo{}
		err := NewSettlementRecorder(repo).RecordPayment(context.Background(), c.org, c.inv, c.ref, c.amount, c.paidAt)
		if !errors.Is(err, ErrInvalidSettlementEntry) || len(repo.rows) != 0 {
			t.Fatalf("%s: err=%v rows=%d", c.name, err, len(repo.rows))
		}
	}
}

func TestSettlementRecorderIdempotentAndPropagatesRepoError(t *testing.T) {
	repo := &fakeSettlementRepo{}
	rec := NewSettlementRecorder(repo)
	for i := 0; i < 2; i++ {
		if err := rec.RecordPayment(context.Background(), ledgerOrg, "inv-1", "req-1", "10.00", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(repo.rows))
	}
	repo.err = errors.New("db down")
	if err := rec.RecordPayment(context.Background(), ledgerOrg, "inv-2", "req-2", "10.00", time.Now()); err == nil {
		t.Fatal("repository error must be returned")
	}
}

func TestSettlementPaymentReference(t *testing.T) {
	if got := settlementPaymentReference(DokuNotification{InvoiceNumber: "RCV-inv-1", ProviderReference: " req-1 "}, "pay-1"); got != "req-1" {
		t.Fatalf("got %q", got)
	}
	if got := settlementPaymentReference(DokuNotification{InvoiceNumber: "RCV-inv-1"}, "pay-1"); got != "RCV-inv-1:pay-1" {
		t.Fatalf("fallback got %q", got)
	}
}
