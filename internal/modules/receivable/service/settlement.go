package service

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"zyad.cloud/internal/modules/receivable/repository"
)

// ErrInvalidSettlementEntry: input entri ledger tidak lengkap atau nominal bukan desimal positif.
var ErrInvalidSettlementEntry = errors.New("invalid settlement entry")

// SettlementRecorder mencatat hak organisasi atas pembayaran online (DOKU) yang masuk merchant
// Zyad: nominal penuh, tanpa potongan. amount berupa string desimal (mis. "150000.00"), bukan float.
// Idempoten per providerReference.
type SettlementRecorder interface {
	RecordPayment(ctx context.Context, orgID, invoiceID, providerReference, amount string, paidAt time.Time) error
}

type settlementRecorder struct {
	repo repository.SettlementRepository
}

func NewSettlementRecorder(repo repository.SettlementRepository) SettlementRecorder {
	return &settlementRecorder{repo: repo}
}

func (s *settlementRecorder) RecordPayment(ctx context.Context, orgID, invoiceID, providerReference, amount string, paidAt time.Time) error {
	orgID, invoiceID = strings.TrimSpace(orgID), strings.TrimSpace(invoiceID)
	providerReference, amount = strings.TrimSpace(providerReference), strings.TrimSpace(amount)
	if orgID == "" || invoiceID == "" || providerReference == "" || paidAt.IsZero() || !validAmount(amount) {
		return ErrInvalidSettlementEntry
	}
	entry := repository.SettlementPaymentParams{
		BeneficiaryOrganizationID: orgID, InvoiceID: invoiceID, ProviderReference: providerReference,
		Amount: amount, OccurredAt: paidAt.UTC(),
	}
	created, err := s.repo.InsertPayment(ctx, entry)
	if err != nil || created {
		return err
	}
	return s.checkDuplicate(ctx, entry)
}

// checkDuplicate: referensi sudah tercatat. Normalnya notifikasi ulang untuk payment yang sama;
// bila org/invoice/amount berbeda, referensi bertabrakan (kunci unik global, payment per org) —
// entri kedua tidak dicatat dan perlu ditangani manual, jadi di-log ERROR.
func (s *settlementRecorder) checkDuplicate(ctx context.Context, entry repository.SettlementPaymentParams) error {
	existing, err := s.repo.FindPaymentByReference(ctx, entry.ProviderReference)
	if err != nil {
		return err
	}
	if existing.BeneficiaryOrganizationID == entry.BeneficiaryOrganizationID && existing.InvoiceID == entry.InvoiceID &&
		sameAmount(existing.Amount, entry.Amount) {
		return nil
	}
	slog.ErrorContext(ctx, "receivable: settlement provider reference collision, entry not recorded",
		"provider_reference", entry.ProviderReference,
		"existing_organization_id", existing.BeneficiaryOrganizationID, "existing_invoice_id", existing.InvoiceID, "existing_amount", existing.Amount,
		"organization_id", entry.BeneficiaryOrganizationID, "invoice_id", entry.InvoiceID, "amount", entry.Amount)
	return nil
}

func sameAmount(a, b string) bool {
	x, ok1 := new(big.Rat).SetString(a)
	y, ok2 := new(big.Rat).SetString(b)
	return ok1 && ok2 && x.Cmp(y) == 0
}

// settlementPaymentReference: kunci idempotensi entri ledger. Referensi DOKU bila ada; bila
// kosong, "<nomor DOKU>:<id receivable_payment>" — deterministik karena RecordProvider
// mengembalikan baris payment yang sama untuk notifikasi ulang, dan unik global karena
// nomor DOKU memuat id invoice.
func settlementPaymentReference(n DokuNotification, paymentID string) string {
	if ref := strings.TrimSpace(n.ProviderReference); ref != "" {
		return ref
	}
	return n.InvoiceNumber + ":" + paymentID
}

// Galat ledger manual (dipetakan ke HTTP di handler).
var (
	ErrSettlementExceedsBalance      = errors.New("settlement exceeds remaining balance")
	ErrAdjustmentBelowZero           = errors.New("adjustment would drive remaining balance below zero")
	ErrAdjustmentNoteRequired        = errors.New("adjustment note is required")
	ErrSettlementBeneficiaryNotFound = errors.New("settlement beneficiary not found")
	ErrSettlementBeneficiaryInvalid  = errors.New("settlement beneficiary must be a customer organization")
)

const (
	settlementKindSettlement = "settlement"
	settlementKindAdjustment = "adjustment"
	settlementNoteMax        = 500
)

// SettlementBalance: masuk (paid_in), disetor (settled), penyesuaian bersih (adjusted), dan
// sisa yang masih menjadi hak org = paid_in - settled + adjusted. String desimal 2 digit.
type SettlementBalance struct{ PaidIn, Settled, Adjusted, Remaining string }

type OrgBalance struct {
	OrganizationID, OrganizationName string
	SettlementBalance
}

type SettlementBalanceFilter struct {
	Search        string
	Limit, Offset int
}

// SettlementEntryInput: Kind hanya 'settlement' atau 'adjustment'. Amount string desimal;
// adjustment boleh diawali '-'. OccurredAt kosong = sekarang (UTC).
type SettlementEntryInput struct {
	Kind, Amount, Note string
	OccurredAt         time.Time
}

type SettlementService interface {
	Balance(ctx context.Context, orgID string) (SettlementBalance, error)
	ListBalances(ctx context.Context, f SettlementBalanceFilter) ([]OrgBalance, int64, error)
	ListEntries(ctx context.Context, orgID string, limit, offset int) ([]repository.SettlementEntry, int64, error)
	Record(ctx context.Context, orgID string, in SettlementEntryInput, actorUserID string) (repository.SettlementEntry, error)
}

type settlementService struct {
	repo repository.SettlementLedgerRepository
	now  func() time.Time
}

func NewSettlementService(repo repository.SettlementLedgerRepository) SettlementService {
	return &settlementService{repo: repo, now: time.Now}
}

func newBalance(t repository.SettlementTotals) (SettlementBalance, error) {
	in, ok1 := new(big.Rat).SetString(t.PaidIn)
	out, ok2 := new(big.Rat).SetString(t.Settled)
	adj, ok3 := new(big.Rat).SetString(t.Adjusted)
	if !ok1 || !ok2 || !ok3 {
		return SettlementBalance{}, errors.New("settlement: malformed totals from repository")
	}
	rem := new(big.Rat).Sub(in, out)
	rem.Add(rem, adj)
	return SettlementBalance{PaidIn: in.FloatString(2), Settled: out.FloatString(2), Adjusted: adj.FloatString(2), Remaining: rem.FloatString(2)}, nil
}

func (s *settlementService) Balance(ctx context.Context, orgID string) (SettlementBalance, error) {
	t, err := s.repo.Totals(ctx, orgID)
	if err != nil {
		return SettlementBalance{}, err
	}
	return newBalance(t)
}

func (s *settlementService) ListBalances(ctx context.Context, f SettlementBalanceFilter) ([]OrgBalance, int64, error) {
	rows, total, err := s.repo.ListBalances(ctx, repository.SettlementBalanceFilter{Search: f.Search, Limit: f.Limit, Offset: f.Offset})
	if err != nil {
		return nil, 0, err
	}
	out := make([]OrgBalance, 0, len(rows))
	for _, r := range rows {
		b, err := newBalance(r.SettlementTotals)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, OrgBalance{OrganizationID: r.OrganizationID, OrganizationName: r.OrganizationName, SettlementBalance: b})
	}
	return out, total, nil
}

func (s *settlementService) ListEntries(ctx context.Context, orgID string, limit, offset int) ([]repository.SettlementEntry, int64, error) {
	return s.repo.ListEntries(ctx, orgID, limit, offset)
}

// signedAmount memvalidasi nominal ledger: desimal maksimal 2 digit, bukan nol. Hanya adjustment
// boleh negatif. Mengembalikan nilai big.Rat untuk perhitungan (tidak pernah float).
func signedAmount(v string, allowNegative bool) (*big.Rat, bool) {
	body := v
	neg := strings.HasPrefix(v, "-")
	if neg {
		if !allowNegative {
			return nil, false
		}
		body = v[1:]
	}
	if !validAmount(body) {
		return nil, false
	}
	r, _ := new(big.Rat).SetString(body)
	if neg {
		r.Neg(r)
	}
	return r, true
}

// Record mencatat setoran manual atau penyesuaian. Cek saldo dilakukan di dalam callback yang
// dijalankan repository di bawah advisory lock org + transaksi yang sama dengan INSERT, jadi dua
// setoran paralel tidak bisa sama-sama lolos (tidak ada read-then-write terpisah di service).
func (s *settlementService) Record(ctx context.Context, orgID string, in SettlementEntryInput, actorUserID string) (repository.SettlementEntry, error) {
	orgID, actorUserID = strings.TrimSpace(orgID), strings.TrimSpace(actorUserID)
	kind, note, amountText := strings.TrimSpace(in.Kind), strings.TrimSpace(in.Note), strings.TrimSpace(in.Amount)
	if orgID == "" || (kind != settlementKindSettlement && kind != settlementKindAdjustment) || utf8.RuneCountInString(note) > settlementNoteMax {
		return repository.SettlementEntry{}, ErrInvalidSettlementEntry
	}
	amount, ok := signedAmount(amountText, kind == settlementKindAdjustment)
	if !ok {
		return repository.SettlementEntry{}, ErrInvalidSettlementEntry
	}
	if kind == settlementKindAdjustment && note == "" {
		return repository.SettlementEntry{}, ErrAdjustmentNoteRequired
	}
	if err := s.requireCustomerBeneficiary(ctx, orgID); err != nil {
		return repository.SettlementEntry{}, err
	}
	occurredAt := in.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = s.now()
	}
	check := func(t repository.SettlementTotals) error {
		bal, err := newBalance(t)
		if err != nil {
			return err
		}
		remaining, _ := new(big.Rat).SetString(bal.Remaining)
		if kind == settlementKindSettlement {
			if amount.Cmp(remaining) > 0 {
				return ErrSettlementExceedsBalance
			}
			return nil
		}
		if new(big.Rat).Add(remaining, amount).Sign() < 0 {
			return ErrAdjustmentBelowZero
		}
		return nil
	}
	return s.repo.InsertManualLocked(ctx, repository.SettlementManualEntry{
		OrganizationID: orgID, Kind: kind, Amount: amount.FloatString(2), Note: note, CreatedBy: actorUserID,
		OccurredAt: occurredAt.UTC(),
	}, check)
}

func (s *settlementService) requireCustomerBeneficiary(ctx context.Context, orgID string) error {
	t, err := s.repo.BeneficiaryType(ctx, orgID)
	if errors.Is(err, repository.ErrOrganizationNotFound) {
		return ErrSettlementBeneficiaryNotFound
	}
	if err != nil {
		return err
	}
	if t != "customer" {
		return ErrSettlementBeneficiaryInvalid
	}
	return nil
}
