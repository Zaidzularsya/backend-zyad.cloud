package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/platform/database"
)

type sendRepository struct{ db *database.Pool }

func NewSendRepository(db *database.Pool) SendRepository { return &sendRepository{db: db} }

const sendSelect = `
	SELECT s.id, s.invoice_id, s.channel, s.recipient, s.status, s.error, s.external_message_id,
		COALESCE(s.client_request_id, ''), s.trigger, COALESCE(s.sent_by::text, ''), COALESCE(u.name, ''), s.sent_at
	FROM receivable_invoice_sends s
	LEFT JOIN users u ON u.id = s.sent_by`

func scanSend(row pgx.Row) (domain.Send, error) {
	var s domain.Send
	err := row.Scan(&s.ID, &s.InvoiceID, &s.Channel, &s.Recipient, &s.Status, &s.Error, &s.ExternalMessageID,
		&s.ClientRequestID, &s.Trigger, &s.SentBy, &s.SentByName, &s.SentAt)
	return s, err
}

func (r *sendRepository) Record(ctx context.Context, scope coretenant.Scope, p RecordSendParams) (domain.Send, error) {
	var out domain.Send
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		trigger := p.Trigger
		if trigger == "" {
			trigger = "manual"
		}
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO receivable_invoice_sends (organization_id, invoice_id, channel, recipient, status, error,
				external_message_id, client_request_id, trigger, sent_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
			scope.OrganizationID(), p.InvoiceID, p.Channel, p.Recipient, p.Status, p.Error, p.ExternalMessageID,
			nullableString(p.ClientRequestID), trigger, nullableString(p.SentBy)).Scan(&id); err != nil {
			return err
		}
		var err error
		out, err = scanSend(tx.QueryRow(ctx, sendSelect+` WHERE s.id = $1 AND s.organization_id = $2`, id, scope.OrganizationID()))
		return err
	})
	return out, err
}

func (r *sendRepository) FindByClientRequest(ctx context.Context, scope coretenant.Scope, invoiceID, clientRequestID string) (domain.Send, error) {
	var out domain.Send
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = scanSend(tx.QueryRow(ctx, sendSelect+`
			WHERE s.organization_id = $1 AND s.invoice_id = $2 AND s.client_request_id = $3`,
			scope.OrganizationID(), invoiceID, clientRequestID))
		return err
	})
	return out, err
}

func (r *sendRepository) ListByInvoice(ctx context.Context, scope coretenant.Scope, invoiceID string) ([]domain.Send, error) {
	out := []domain.Send{}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sendSelect+`
			WHERE s.organization_id = $1 AND s.invoice_id = $2 ORDER BY s.sent_at DESC, s.id DESC`,
			scope.OrganizationID(), invoiceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			s, err := scanSend(rows)
			if err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	return out, err
}

func (r *sendRepository) LatestFailedByInvoice(ctx context.Context, scope coretenant.Scope, invoiceID string) (domain.Send, error) {
	var out domain.Send
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = scanSend(tx.QueryRow(ctx, sendSelect+`
			WHERE s.organization_id = $1 AND s.invoice_id = $2 AND s.status = 'failed'
				AND NOT EXISTS (
					SELECT 1 FROM receivable_invoice_sends ok
					WHERE ok.organization_id = s.organization_id AND ok.invoice_id = s.invoice_id
						AND ok.channel = s.channel AND ok.status = 'sent' AND ok.sent_at >= s.sent_at)
			ORDER BY s.sent_at DESC, s.id DESC LIMIT 1`, scope.OrganizationID(), invoiceID))
		return err
	})
	return out, err
}

func (r *sendRepository) LatestByInvoices(ctx context.Context, scope coretenant.Scope, invoiceIDs []string) (map[string]domain.Send, error) {
	out := map[string]domain.Send{}
	if len(invoiceIDs) == 0 {
		return out, nil
	}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT ON (s.invoice_id) s.id, s.invoice_id, s.channel, s.recipient, s.status, s.error, s.external_message_id,
				COALESCE(s.client_request_id, ''), s.trigger, COALESCE(s.sent_by::text, ''), COALESCE(u.name, ''), s.sent_at
			FROM receivable_invoice_sends s
			LEFT JOIN users u ON u.id = s.sent_by
			WHERE s.organization_id = $1 AND s.invoice_id = ANY($2::uuid[])
			ORDER BY s.invoice_id, s.sent_at DESC, s.id DESC`, scope.OrganizationID(), invoiceIDs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			s, err := scanSend(rows)
			if err != nil {
				return err
			}
			out[s.InvoiceID] = s
		}
		return rows.Err()
	})
	return out, err
}
