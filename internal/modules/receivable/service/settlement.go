package service

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"strings"
	"time"

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
