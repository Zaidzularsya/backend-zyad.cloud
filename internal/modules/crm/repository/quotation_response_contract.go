package repository

import (
	"context"
	"errors"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

var ErrQuotationNotRespondable = errors.New("quotation is not awaiting a customer response")

type RecordResponseParams struct {
	QuotationID, LinkID, Note, ResponderName, IP, UserAgent string
	Action                                                  domain.QuotationResponseAction
	Categories                                              []domain.RevisionCategory
}

type QuotationResponseRepository interface {
	// Respond mengubah status sent → approved|revision_requested dan mencatat
	// respons dalam satu transaksi. Status bukan sent → ErrQuotationNotRespondable
	// (tanpa menulis apa pun). approved juga mengisi approved_at.
	Respond(ctx context.Context, scope coretenant.Scope, p RecordResponseParams) (domain.Quotation, domain.QuotationResponse, error)
	ListByQuotation(ctx context.Context, scope coretenant.Scope, quotationID string) ([]domain.QuotationResponse, error) // terbaru dulu
}
