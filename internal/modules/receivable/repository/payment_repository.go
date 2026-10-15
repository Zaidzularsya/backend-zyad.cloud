package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/platform/database"
)

type paymentRepository struct{ db *database.Pool }

func NewPaymentRepository(db *database.Pool) PaymentRepository { return &paymentRepository{db: db} }

const paymentColumns = `p.id, p.invoice_id, p.amount::text, p.method, COALESCE(p.reference, ''), p.note,
	COALESCE(p.recorded_by::text, ''), p.paid_at, p.created_at`

func scanPayment(row pgx.Row) (domain.Payment, error) {
	var p domain.Payment
	err := row.Scan(&p.ID, &p.InvoiceID, &p.Amount, &p.Method, &p.Reference, &p.Note, &p.RecordedBy, &p.PaidAt, &p.CreatedAt)
	return p, err
}

func (r *paymentRepository) Record(ctx context.Context, scope coretenant.Scope, invoiceID string, p PaymentParams) (domain.Invoice, domain.Payment, bool, error) {
	var inv domain.Invoice
	var pay domain.Payment
	created := false
	orgID := scope.OrganizationID()
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		// Kunci baris invoice: pembayaran bersamaan diserialkan sehingga total tidak bisa terlewati.
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM receivable_invoices WHERE id = $1 AND organization_id = $2 FOR UPDATE`,
			invoiceID, orgID).Scan(&status); err != nil {
			return err
		}
		// Hanya referensi provider yang idempoten; referensi manual adalah catatan bebas.
		if p.Reference != "" && p.Method != "manual" {
			existing, err := scanPayment(tx.QueryRow(ctx, `SELECT `+paymentColumns+` FROM receivable_payments p
				WHERE p.organization_id = $1 AND p.method = $2 AND p.reference = $3`, orgID, p.Method, p.Reference))
			if err == nil {
				pay = existing
				inv, err = findInTx(ctx, tx, orgID, `i.id = $2`, existing.InvoiceID)
				return err
			}
			if err != pgx.ErrNoRows {
				return err
			}
		}
		if status != string(domain.InvoiceIssued) && status != string(domain.InvoiceOverdue) {
			return ErrInvoiceNotPayable
		}
		var fits bool
		if err := tx.QueryRow(ctx, `SELECT $3::numeric <= grand_total - amount_paid FROM receivable_invoices WHERE id = $1 AND organization_id = $2`,
			invoiceID, orgID, p.Amount).Scan(&fits); err != nil {
			return err
		}
		if !fits {
			return ErrPaymentExceeds
		}
		var payload any
		if len(p.ProviderPayload) > 0 {
			payload = p.ProviderPayload
		}
		var err error
		pay, err = scanPayment(tx.QueryRow(ctx, `
			INSERT INTO receivable_payments AS p (organization_id, invoice_id, amount, method, reference, note, provider_payload, recorded_by, paid_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING `+paymentColumns, orgID, invoiceID, p.Amount, p.Method, nullableString(p.Reference), p.Note,
			payload, nullableString(p.RecordedBy), p.PaidAt))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE receivable_invoices SET
				amount_paid = amount_paid + $3::numeric,
				status = CASE WHEN amount_paid + $3::numeric = grand_total THEN 'paid' ELSE status END,
				paid_at = CASE WHEN amount_paid + $3::numeric = grand_total THEN $4 ELSE paid_at END,
				updated_at = now()
			WHERE id = $1 AND organization_id = $2`, invoiceID, orgID, p.Amount, p.PaidAt); err != nil {
			return err
		}
		created = true
		inv, err = findInTx(ctx, tx, orgID, `i.id = $2`, invoiceID)
		return err
	})
	if err != nil {
		return domain.Invoice{}, domain.Payment{}, false, err
	}
	return inv, pay, created, nil
}

func (r *paymentRepository) List(ctx context.Context, scope coretenant.Scope, invoiceID string) ([]domain.Payment, error) {
	out := []domain.Payment{}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+paymentColumns+` FROM receivable_payments p
			WHERE p.organization_id = $1 AND p.invoice_id = $2 ORDER BY p.paid_at, p.created_at, p.id`, scope.OrganizationID(), invoiceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanPayment(rows)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

func (r *paymentRepository) ListAll(ctx context.Context, scope coretenant.Scope, limit, offset int) ([]domain.Payment, int64, error) {
	out := []domain.Payment{}
	var total int64
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM receivable_payments WHERE organization_id = $1`, scope.OrganizationID()).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+paymentColumns+`, COALESCE(i.invoice_number, ''), a.name
			FROM receivable_payments p
			JOIN receivable_invoices i ON i.organization_id = p.organization_id AND i.id = p.invoice_id
			JOIN receivable_accounts a ON a.organization_id = i.organization_id AND a.id = i.account_id
			WHERE p.organization_id = $1 ORDER BY p.paid_at DESC, p.created_at DESC, p.id DESC LIMIT $2 OFFSET $3`,
			scope.OrganizationID(), clampLimit(limit), max(offset, 0))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p domain.Payment
			if err := rows.Scan(&p.ID, &p.InvoiceID, &p.Amount, &p.Method, &p.Reference, &p.Note, &p.RecordedBy, &p.PaidAt, &p.CreatedAt,
				&p.InvoiceNumber, &p.AccountName); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, total, err
}
