package service

import (
	"context"
	"errors"
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
	_, err := s.repo.InsertPayment(ctx, repository.SettlementPaymentParams{
		BeneficiaryOrganizationID: orgID, InvoiceID: invoiceID, ProviderReference: providerReference,
		Amount: amount, OccurredAt: paidAt.UTC(),
	})
	return err
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
