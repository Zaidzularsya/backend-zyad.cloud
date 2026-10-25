package repository

import (
	"context"
	"time"

	"zyad.cloud/internal/platform/database"
)

// SettlementPaymentParams adalah satu entri pemasukan (kind=payment) ledger penyelesaian.
// Amount berupa string desimal (numeric), tidak pernah float.
type SettlementPaymentParams struct {
	BeneficiaryOrganizationID, InvoiceID, ProviderReference, Amount string
	OccurredAt                                                      time.Time
}

// SettlementRepository menulis ke settlement_entries: pembukuan lintas tenant milik
// platform, tanpa RLS (lihat migration 000165). Pembatasan akses ada di service.
type SettlementRepository interface {
	// InsertPayment idempoten per provider_reference; created=false bila sudah ada.
	InsertPayment(ctx context.Context, p SettlementPaymentParams) (bool, error)
}

type settlementRepository struct{ db *database.Pool }

func NewSettlementRepository(db *database.Pool) SettlementRepository {
	return &settlementRepository{db: db}
}

func (r *settlementRepository) InsertPayment(ctx context.Context, p SettlementPaymentParams) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO settlement_entries (beneficiary_organization_id, kind, amount, invoice_id, provider_reference, occurred_at)
		VALUES ($1, 'payment', $2::numeric, $3, $4, $5)
		ON CONFLICT (kind, provider_reference) WHERE kind = 'payment' DO NOTHING`,
		p.BeneficiaryOrganizationID, p.Amount, p.InvoiceID, p.ProviderReference, p.OccurredAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
