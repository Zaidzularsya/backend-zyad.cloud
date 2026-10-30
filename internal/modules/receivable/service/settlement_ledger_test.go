package service

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/receivable/repository"
)

const (
	orgA     = "11111111-1111-1111-1111-111111111111"
	orgB     = "22222222-2222-2222-2222-222222222222"
	orgPlat  = "99999999-9999-9999-9999-999999999999"
	actorID  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	orgGhost = "00000000-0000-0000-0000-000000000000"
)

// fakeLedgerRepo meniru kontrak SettlementLedgerRepository: InsertManualLocked membaca total,
// menjalankan check, dan menyisipkan dalam satu langkah yang diserialkan oleh mutex (pengganti
// advisory lock). calls mencatat urutan operasi supaya test bisa menegaskan tidak ada
// read-then-write terpisah.
type fakeLedgerRepo struct {
	types   map[string]string
	entries []repository.SettlementEntry
	calls   []string
}

func (f *fakeLedgerRepo) totals(orgID string) repository.SettlementTotals {
	sum := map[string]*big.Rat{"payment": new(big.Rat), "settlement": new(big.Rat), "adjustment": new(big.Rat)}
	for _, e := range f.entries {
		if e.OrganizationID != orgID {
			continue
		}
		r, _ := new(big.Rat).SetString(e.Amount)
		sum[e.Kind].Add(sum[e.Kind], r)
	}
	return repository.SettlementTotals{PaidIn: sum["payment"].FloatString(2), Settled: sum["settlement"].FloatString(2), Adjusted: sum["adjustment"].FloatString(2)}
}

func (f *fakeLedgerRepo) Totals(_ context.Context, orgID string) (repository.SettlementTotals, error) {
	f.calls = append(f.calls, "totals")
	return f.totals(orgID), nil
}

func (f *fakeLedgerRepo) BeneficiaryType(_ context.Context, orgID string) (string, error) {
	f.calls = append(f.calls, "type")
	t, ok := f.types[orgID]
	if !ok {
		return "", repository.ErrOrganizationNotFound
	}
	return t, nil
}

func (f *fakeLedgerRepo) ListBalances(_ context.Context, _ repository.SettlementBalanceFilter) ([]repository.SettlementOrgBalance, int64, error) {
	return []repository.SettlementOrgBalance{{OrganizationID: orgA, OrganizationName: "Acme", SettlementTotals: f.totals(orgA)}}, 1, nil
}

func (f *fakeLedgerRepo) ListEntries(_ context.Context, orgID string, _, _ int) ([]repository.SettlementEntry, int64, error) {
	var out []repository.SettlementEntry
	for _, e := range f.entries {
		if e.OrganizationID == orgID {
			out = append(out, e)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeLedgerRepo) InsertManualLocked(_ context.Context, in repository.SettlementManualEntry, check func(repository.SettlementTotals) error) (repository.SettlementEntry, error) {
	f.calls = append(f.calls, "lock", "totals-in-lock")
	if err := check(f.totals(in.OrganizationID)); err != nil {
		return repository.SettlementEntry{}, err
	}
	f.calls = append(f.calls, "insert")
	e := repository.SettlementEntry{ID: "e", OrganizationID: in.OrganizationID, Kind: in.Kind, Amount: in.Amount, Note: in.Note, OccurredAt: in.OccurredAt, CreatedBy: in.CreatedBy}
	f.entries = append(f.entries, e)
	return e, nil
}

func (f *fakeLedgerRepo) add(org, kind, amount string) {
	f.entries = append(f.entries, repository.SettlementEntry{OrganizationID: org, Kind: kind, Amount: amount})
}

func newLedger() (*fakeLedgerRepo, SettlementService) {
	f := &fakeLedgerRepo{types: map[string]string{orgA: "customer", orgB: "customer", orgPlat: "platform"}}
	return f, NewSettlementService(f)
}

func TestSettlementBalanceFormula(t *testing.T) {
	f, svc := newLedger()
	f.add(orgA, "payment", "150000.50")
	f.add(orgA, "payment", "0.25")
	f.add(orgA, "settlement", "50000.00")
	f.add(orgA, "adjustment", "-0.75")
	f.add(orgA, "adjustment", "10.00")
	f.add(orgB, "payment", "999.00") // tidak boleh bocor ke A
	b, err := svc.Balance(context.Background(), orgA)
	if err != nil {
		t.Fatal(err)
	}
	want := SettlementBalance{PaidIn: "150000.75", Settled: "50000.00", Adjusted: "9.25", Remaining: "100010.00"}
	if b != want {
		t.Fatalf("balance = %+v, want %+v", b, want)
	}
}

func TestSettlementBalanceEmptyIsZero(t *testing.T) {
	_, svc := newLedger()
	b, err := svc.Balance(context.Background(), orgA)
	if err != nil || b.Remaining != "0.00" || b.PaidIn != "0.00" {
		t.Fatalf("balance = %+v err=%v", b, err)
	}
}

func TestSettlementRecord(t *testing.T) {
	now := time.Date(2026, 10, 25, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		org     string
		in      SettlementEntryInput
		wantErr error
	}{
		{"setoran kurang dari saldo", orgA, SettlementEntryInput{Kind: "settlement", Amount: "40000.00"}, nil},
		{"setoran sama dengan saldo", orgA, SettlementEntryInput{Kind: "settlement", Amount: "100000.50"}, nil},
		{"setoran melebihi saldo", orgA, SettlementEntryInput{Kind: "settlement", Amount: "100000.51"}, ErrSettlementExceedsBalance},
		{"setoran nol", orgA, SettlementEntryInput{Kind: "settlement", Amount: "0"}, ErrInvalidSettlementEntry},
		{"setoran negatif", orgA, SettlementEntryInput{Kind: "settlement", Amount: "-5"}, ErrInvalidSettlementEntry},
		{"setoran 3 desimal", orgA, SettlementEntryInput{Kind: "settlement", Amount: "1.234"}, ErrInvalidSettlementEntry},
		{"kind payment ditolak", orgA, SettlementEntryInput{Kind: "payment", Amount: "1.00"}, ErrInvalidSettlementEntry},
		{"kind kosong ditolak", orgA, SettlementEntryInput{Kind: "", Amount: "1.00"}, ErrInvalidSettlementEntry},
		{"penyesuaian tanpa catatan", orgA, SettlementEntryInput{Kind: "adjustment", Amount: "5.00"}, ErrAdjustmentNoteRequired},
		{"penyesuaian catatan spasi", orgA, SettlementEntryInput{Kind: "adjustment", Amount: "5.00", Note: "  "}, ErrAdjustmentNoteRequired},
		{"penyesuaian nol", orgA, SettlementEntryInput{Kind: "adjustment", Amount: "0.00", Note: "x"}, ErrInvalidSettlementEntry},
		{"penyesuaian positif", orgA, SettlementEntryInput{Kind: "adjustment", Amount: "5.25", Note: "refund manual"}, nil},
		{"penyesuaian negatif dalam saldo", orgA, SettlementEntryInput{Kind: "adjustment", Amount: "-100000.50", Note: "koreksi"}, nil},
		{"penyesuaian negatif di bawah nol", orgA, SettlementEntryInput{Kind: "adjustment", Amount: "-100000.51", Note: "koreksi"}, ErrAdjustmentBelowZero},
		{"catatan terlalu panjang", orgA, SettlementEntryInput{Kind: "adjustment", Amount: "1", Note: strings.Repeat("x", 501)}, ErrInvalidSettlementEntry},
		{"org platform ditolak", orgPlat, SettlementEntryInput{Kind: "settlement", Amount: "1"}, ErrSettlementBeneficiaryInvalid},
		{"org tidak ada", orgGhost, SettlementEntryInput{Kind: "settlement", Amount: "1"}, ErrSettlementBeneficiaryNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, svc := newLedger()
			svc.(*settlementService).now = func() time.Time { return now }
			f.add(orgA, "payment", "100000.50")
			e, err := svc.Record(context.Background(), tc.org, tc.in, actorID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if len(f.entries) != 1 {
					t.Fatalf("entri ditulis walau ditolak: %d", len(f.entries))
				}
				return
			}
			if e.OccurredAt != now || e.CreatedBy != actorID {
				t.Fatalf("entry = %+v", e)
			}
		})
	}
}

func TestSettlementEqualToBalanceLeavesZero(t *testing.T) {
	f, svc := newLedger()
	f.add(orgA, "payment", "150000.50")
	if _, err := svc.Record(context.Background(), orgA, SettlementEntryInput{Kind: "settlement", Amount: "150000.50"}, actorID); err != nil {
		t.Fatal(err)
	}
	b, _ := svc.Balance(context.Background(), orgA)
	if b.Remaining != "0.00" || b.Settled != "150000.50" {
		t.Fatalf("balance = %+v", b)
	}
}

// Dua setoran berurutan yang masing-masing muat sendiri tetapi tidak muat bersama: yang kedua
// harus ditolak karena pengecekan memakai total di dalam langkah terkunci, bukan snapshot lama.
func TestSettlementSecondSettlementSeesFirst(t *testing.T) {
	f, svc := newLedger()
	f.add(orgA, "payment", "100.00")
	if _, err := svc.Record(context.Background(), orgA, SettlementEntryInput{Kind: "settlement", Amount: "60.00"}, actorID); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Record(context.Background(), orgA, SettlementEntryInput{Kind: "settlement", Amount: "60.00"}, actorID)
	if !errors.Is(err, ErrSettlementExceedsBalance) {
		t.Fatalf("err = %v", err)
	}
}

// Pengecekan saldo dan insert harus lewat satu panggilan repository terkunci; service tidak boleh
// membaca Totals terpisah sebelum menulis (itu membuka celah race).
func TestSettlementRecordUsesLockedCheckOnly(t *testing.T) {
	f, svc := newLedger()
	f.add(orgA, "payment", "100.00")
	if _, err := svc.Record(context.Background(), orgA, SettlementEntryInput{Kind: "settlement", Amount: "10.00"}, actorID); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if c == "totals" {
			t.Fatalf("Record membaca Totals di luar kunci: %v", f.calls)
		}
	}
	if got := strings.Join(f.calls, ","); got != "type,lock,totals-in-lock,insert" {
		t.Fatalf("calls = %s", got)
	}
}

func TestSettlementListBalancesAndEntries(t *testing.T) {
	f, svc := newLedger()
	f.add(orgA, "payment", "10.00")
	f.add(orgB, "payment", "20.00")
	bals, total, err := svc.ListBalances(context.Background(), SettlementBalanceFilter{Limit: 10})
	if err != nil || total != 1 || len(bals) != 1 || bals[0].Remaining != "10.00" || bals[0].OrganizationName != "Acme" {
		t.Fatalf("bals=%+v total=%d err=%v", bals, total, err)
	}
	entries, _, err := svc.ListEntries(context.Background(), orgA, 50, 0)
	if err != nil || len(entries) != 1 || entries[0].OrganizationID != orgA {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
}
