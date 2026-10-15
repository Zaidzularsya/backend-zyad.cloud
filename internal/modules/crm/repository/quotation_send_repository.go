package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
)

type quotationSendRepository struct{ db *database.Pool }

func NewQuotationSendRepository(db *database.Pool) QuotationSendRepository {
	return &quotationSendRepository{db: db}
}

const quotationSendSelect = `
	SELECT s.id, s.quotation_id, s.channel, s.mode, s.recipient, s.pdf_asset_id, s.status,
		COALESCE(s.error, ''), COALESCE(s.external_message_id, ''), COALESCE(s.client_request_id, ''),
		COALESCE(s.sent_by::text, ''), COALESCE(u.name, ''), s.sent_at
	FROM crm_quotation_sends s
	LEFT JOIN users u ON u.id = s.sent_by`

func scanQuotationSend(row pgx.Row) (domain.QuotationSend, error) {
	var s domain.QuotationSend
	var channel, mode, status string
	err := row.Scan(&s.ID, &s.QuotationID, &channel, &mode, &s.Recipient, &s.PDFAssetID, &status,
		&s.Error, &s.ExternalMessageID, &s.ClientRequestID, &s.SentBy, &s.SentByName, &s.SentAt)
	s.Channel, s.Mode, s.Status = domain.QuotationSendChannel(channel), domain.QuotationSendMode(mode), domain.QuotationSendStatus(status)
	return s, err
}

func (r *quotationSendRepository) Record(ctx context.Context, scope coretenant.Scope, p RecordQuotationSendParams) (domain.QuotationSend, error) {
	var out domain.QuotationSend
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO crm_quotation_sends (organization_id, quotation_id, channel, mode, recipient, pdf_asset_id,
				status, error, external_message_id, client_request_id, sent_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
			scope.OrganizationID(), p.QuotationID, string(p.Channel), string(p.Mode), p.Recipient, nullableString(p.PDFAssetID),
			string(p.Status), nullableString(p.Error), nullableString(p.ExternalMessageID), nullableString(p.ClientRequestID),
			nullableString(p.SentBy)).Scan(&id); err != nil {
			return err
		}
		var err error
		out, err = scanQuotationSend(tx.QueryRow(ctx, quotationSendSelect+` WHERE s.id = $1 AND s.organization_id = $2`, id, scope.OrganizationID()))
		return err
	})
	return out, err
}

func (r *quotationSendRepository) FindByClientRequest(ctx context.Context, scope coretenant.Scope, quotationID, clientRequestID string) (domain.QuotationSend, error) {
	var out domain.QuotationSend
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = scanQuotationSend(tx.QueryRow(ctx, quotationSendSelect+`
			WHERE s.organization_id = $1 AND s.quotation_id = $2 AND s.client_request_id = $3`,
			scope.OrganizationID(), quotationID, clientRequestID))
		return err
	})
	return out, err
}

func (r *quotationSendRepository) ListByQuotation(ctx context.Context, scope coretenant.Scope, quotationID string) ([]domain.QuotationSend, error) {
	out := []domain.QuotationSend{}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, quotationSendSelect+`
			WHERE s.organization_id = $1 AND s.quotation_id = $2 ORDER BY s.sent_at DESC, s.id DESC`,
			scope.OrganizationID(), quotationID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			s, err := scanQuotationSend(rows)
			if err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	return out, err
}
