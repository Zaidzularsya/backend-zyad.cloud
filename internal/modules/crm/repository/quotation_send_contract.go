package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type RecordQuotationSendParams struct {
	QuotationID       string
	Recipient         string
	PDFAssetID        string
	Error             string
	ExternalMessageID string
	ClientRequestID   string
	SentBy            string
	Channel           domain.QuotationSendChannel
	Mode              domain.QuotationSendMode
	Status            domain.QuotationSendStatus
}

// QuotationSendRepository mencatat log kiriman quotation. client_request_id
// unik per quotation (idempotensi kirim).
type QuotationSendRepository interface {
	Record(ctx context.Context, scope coretenant.Scope, p RecordQuotationSendParams) (domain.QuotationSend, error)
	// FindByClientRequest mengembalikan pgx.ErrNoRows bila tidak ada.
	FindByClientRequest(ctx context.Context, scope coretenant.Scope, quotationID, clientRequestID string) (domain.QuotationSend, error)
	// ListByQuotation: terbaru dulu.
	ListByQuotation(ctx context.Context, scope coretenant.Scope, quotationID string) ([]domain.QuotationSend, error)
}
