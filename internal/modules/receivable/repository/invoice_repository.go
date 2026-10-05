package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/shared/pricing"
)

type invoiceRepository struct{ db *database.Pool }

func NewInvoiceRepository(db *database.Pool) InvoiceRepository { return &invoiceRepository{db: db} }

const invoiceSelect = `
	SELECT i.id, i.account_id, COALESCE(i.invoice_number, ''), i.status, i.source_type,
		COALESCE(i.source_id::text, ''), COALESCE(i.contract_id::text, ''), COALESCE(i.idempotency_key, ''),
		i.period_start, i.period_end, i.issue_date, i.due_date, i.currency,
		i.subtotal::text, i.discount_total::text, i.tax_total::text, i.grand_total::text, i.amount_paid::text,
		i.channels, COALESCE(i.pic_user_id::text, ''), i.pdf_asset_id::text, i.notes, i.paid_at, i.voided_at,
		i.void_reason, COALESCE(i.created_by::text, ''), i.created_at, i.updated_at,
		a.id, a.name, a.company_name, a.email, a.phone, a.address,
		COALESCE(a.source_type, ''), COALESCE(a.source_id::text, ''), a.created_at, a.updated_at
	FROM receivable_invoices i
	JOIN receivable_accounts a ON a.organization_id = i.organization_id AND a.id = i.account_id`

func scanInvoice(row pgx.Row) (domain.Invoice, error) {
	var inv domain.Invoice
	var status, sourceType string
	err := row.Scan(&inv.ID, &inv.AccountID, &inv.InvoiceNumber, &status, &sourceType,
		&inv.SourceID, &inv.ContractID, &inv.IdempotencyKey,
		&inv.PeriodStart, &inv.PeriodEnd, &inv.IssueDate, &inv.DueDate, &inv.Currency,
		&inv.Subtotal, &inv.DiscountTotal, &inv.TaxTotal, &inv.GrandTotal, &inv.AmountPaid,
		&inv.Channels, &inv.PICUserID, &inv.PDFAssetID, &inv.Notes, &inv.PaidAt, &inv.VoidedAt,
		&inv.VoidReason, &inv.CreatedBy, &inv.CreatedAt, &inv.UpdatedAt,
		&inv.Account.ID, &inv.Account.Name, &inv.Account.CompanyName, &inv.Account.Email, &inv.Account.Phone, &inv.Account.Address,
		&inv.Account.SourceType, &inv.Account.SourceID, &inv.Account.CreatedAt, &inv.Account.UpdatedAt)
	inv.Status, inv.SourceType = domain.InvoiceStatus(status), domain.SourceType(sourceType)
	return inv, err
}

func loadItems(ctx context.Context, tx pgx.Tx, orgID, invoiceID string) ([]domain.InvoiceItem, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, description, quantity::text, unit, unit_price::text, COALESCE(discount_percent::text, ''),
			tax_percent::text, tax_amount::text, line_total::text, COALESCE(product_id::text, ''), sku,
			COALESCE(contract_item_id::text, ''), COALESCE(source_line_id::text, ''),
			charge_type, COALESCE(billing_frequency, ''), payment_timing, period_start, period_end, position
		FROM receivable_invoice_items
		WHERE organization_id = $1 AND invoice_id = $2 ORDER BY position, created_at, id`, orgID, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.InvoiceItem{}
	for rows.Next() {
		var it domain.InvoiceItem
		var charge, freq, timing string
		if err := rows.Scan(&it.ID, &it.Description, &it.Quantity, &it.Unit, &it.UnitPrice, &it.DiscountPercent,
			&it.TaxPercent, &it.TaxAmount, &it.LineTotal, &it.ProductID, &it.SKU, &it.ContractItemID, &it.SourceLineID,
			&charge, &freq, &timing, &it.PeriodStart, &it.PeriodEnd, &it.Position); err != nil {
			return nil, err
		}
		it.Pricing = pricing.Attributes{ChargeType: pricing.ChargeType(charge), Frequency: pricing.Frequency(freq), PaymentTiming: pricing.PaymentTiming(timing)}
		items = append(items, it)
	}
	return items, rows.Err()
}

// findInTx membaca invoice + item di dalam transaksi yang sama.
func findInTx(ctx context.Context, tx pgx.Tx, orgID, where string, args ...any) (domain.Invoice, error) {
	inv, err := scanInvoice(tx.QueryRow(ctx, invoiceSelect+` WHERE i.organization_id = $1 AND `+where, append([]any{orgID}, args...)...))
	if err != nil {
		return domain.Invoice{}, err
	}
	inv.Items, err = loadItems(ctx, tx, orgID, inv.ID)
	return inv, err
}

func insertItems(ctx context.Context, tx pgx.Tx, orgID, invoiceID string, items []InvoiceItemParams) error {
	for i, it := range items {
		l := it.Line
		if _, err := tx.Exec(ctx, `
			INSERT INTO receivable_invoice_items (organization_id, invoice_id, description, quantity, unit, unit_price,
				discount_percent, tax_percent, tax_amount, line_total, product_id, sku, contract_item_id, source_line_id,
				period_start, period_end, charge_type, billing_frequency, payment_timing, position)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`,
			orgID, invoiceID, l.Description, l.Quantity, it.Unit, l.UnitPrice,
			nullableString(l.DiscountPercent), l.TaxPercent, l.TaxAmount, l.LineTotal,
			nullableString(it.ProductID), it.SKU, nullableString(it.ContractItemID), nullableString(it.SourceLineID),
			it.PeriodStart, it.PeriodEnd, string(l.Attrs.ChargeType), nullableString(string(l.Attrs.Frequency)),
			string(l.Attrs.PaymentTiming), i); err != nil {
			return err
		}
	}
	return nil
}

func channelsOrDefault(c []string) []string {
	if len(c) == 0 {
		return []string{"email"}
	}
	return c
}

func (r *invoiceRepository) Create(ctx context.Context, scope coretenant.Scope, p CreateInvoiceParams) (domain.Invoice, error) {
	sourceType := p.SourceType
	if sourceType == "" {
		sourceType = string(domain.SourceManual)
	}
	currency := p.Currency
	if currency == "" {
		currency = "IDR"
	}
	var out domain.Invoice
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO receivable_invoices (organization_id, account_id, source_type, source_id, contract_id, idempotency_key,
				period_start, period_end, currency, subtotal, discount_total, tax_total, grand_total, channels,
				pic_user_id, notes, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $17)
			RETURNING id`,
			scope.OrganizationID(), p.AccountID, sourceType, nullableString(p.SourceID), nullableString(p.ContractID),
			nullableString(p.IdempotencyKey), p.PeriodStart, p.PeriodEnd, currency,
			p.Totals.Subtotal, p.Totals.DiscountTotal, p.Totals.TaxTotal, p.Totals.GrandTotal, channelsOrDefault(p.Channels),
			nullableString(p.PICUserID), p.Notes, nullableString(p.CreatedBy)).Scan(&id); err != nil {
			return err
		}
		if err := insertItems(ctx, tx, scope.OrganizationID(), id, p.Items); err != nil {
			return err
		}
		var err error
		out, err = findInTx(ctx, tx, scope.OrganizationID(), `i.id = $2`, id)
		return err
	})
	if isUniqueViolation(err, "idx_receivable_invoices_source_idempotency") {
		return domain.Invoice{}, ErrDuplicateSource
	}
	return out, err
}

func (r *invoiceRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Invoice, error) {
	var out domain.Invoice
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = findInTx(ctx, tx, scope.OrganizationID(), `i.id = $2`, id)
		return err
	})
	return out, err
}

func (r *invoiceRepository) FindBySource(ctx context.Context, scope coretenant.Scope, sourceType, sourceID, idempotencyKey string) (domain.Invoice, error) {
	var out domain.Invoice
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = findInTx(ctx, tx, scope.OrganizationID(),
			`i.source_type = $2 AND i.source_id = $3 AND i.idempotency_key = $4`, sourceType, sourceID, idempotencyKey)
		return err
	})
	return out, err
}

func (r *invoiceRepository) List(ctx context.Context, scope coretenant.Scope, f InvoiceListFilter) ([]domain.Invoice, int64, error) {
	conds := []string{"i.organization_id = $1"}
	args := []any{scope.OrganizationID()}
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}
	if f.Status != "" {
		add("i.status = ?", f.Status)
	}
	if f.AccountID != "" {
		add("i.account_id = ?", f.AccountID)
	}
	if f.SourceType != "" {
		add("i.source_type = ?", f.SourceType)
	}
	if f.SourceID != "" {
		add("i.source_id = ?", f.SourceID)
	}
	if f.ContractID != "" {
		add("i.contract_id = ?", f.ContractID)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		add("(i.invoice_number ILIKE ? OR a.name ILIKE ? OR a.company_name ILIKE ?)", "%"+escapeLike(s)+"%")
	}
	if f.SendFailed {
		conds = append(conds, `EXISTS (
			SELECT 1 FROM receivable_invoice_sends s
			WHERE s.organization_id = i.organization_id AND s.invoice_id = i.id AND s.status = 'failed'
				AND NOT EXISTS (
					SELECT 1 FROM receivable_invoice_sends ok
					WHERE ok.organization_id = s.organization_id AND ok.invoice_id = s.invoice_id
						AND ok.channel = s.channel AND ok.status = 'sent' AND ok.sent_at >= s.sent_at))`)
	}
	where := strings.Join(conds, " AND ")
	out := []domain.Invoice{}
	var total int64
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM receivable_invoices i
			JOIN receivable_accounts a ON a.organization_id = i.organization_id AND a.id = i.account_id
			WHERE `+where, args...).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, invoiceSelect+` WHERE `+where+fmt.Sprintf(`
			ORDER BY i.created_at DESC, i.id DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2),
			append(args, clampLimit(f.Limit), max(f.Offset, 0))...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			inv, err := scanInvoice(rows)
			if err != nil {
				return err
			}
			out = append(out, inv)
		}
		return rows.Err()
	})
	return out, total, err
}

func (r *invoiceRepository) ReplaceDraft(ctx context.Context, scope coretenant.Scope, id string, p CreateInvoiceParams) (domain.Invoice, error) {
	var out domain.Invoice
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM receivable_invoices WHERE id = $1 AND organization_id = $2 FOR UPDATE`,
			id, scope.OrganizationID()).Scan(&status); err != nil {
			return err
		}
		if status != string(domain.InvoiceDraft) {
			return ErrInvoiceNotDraft
		}
		if _, err := tx.Exec(ctx, `
			UPDATE receivable_invoices SET account_id = $3, period_start = $4, period_end = $5, currency = COALESCE(NULLIF($6, ''), currency),
				subtotal = $7, discount_total = $8, tax_total = $9, grand_total = $10, channels = $11,
				pic_user_id = $12, notes = $13, updated_by = $14, updated_at = now()
			WHERE id = $1 AND organization_id = $2`,
			id, scope.OrganizationID(), p.AccountID, p.PeriodStart, p.PeriodEnd, p.Currency,
			p.Totals.Subtotal, p.Totals.DiscountTotal, p.Totals.TaxTotal, p.Totals.GrandTotal, channelsOrDefault(p.Channels),
			nullableString(p.PICUserID), p.Notes, nullableString(p.CreatedBy)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM receivable_invoice_items WHERE invoice_id = $1 AND organization_id = $2`,
			id, scope.OrganizationID()); err != nil {
			return err
		}
		if err := insertItems(ctx, tx, scope.OrganizationID(), id, p.Items); err != nil {
			return err
		}
		var err error
		out, err = findInTx(ctx, tx, scope.OrganizationID(), `i.id = $2`, id)
		return err
	})
	return out, err
}

func (r *invoiceRepository) Issue(ctx context.Context, scope coretenant.Scope, id, number string, issueDate, dueDate time.Time, by string) (domain.Invoice, error) {
	var out domain.Invoice
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		// Kondisional status = 'draft': dua penerbitan bersamaan hanya satu yang berhasil.
		tag, err := tx.Exec(ctx, `
			UPDATE receivable_invoices SET status = 'issued', invoice_number = $3, issue_date = $4, due_date = $5,
				updated_by = $6, updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND status = 'draft'`,
			id, scope.OrganizationID(), number, issueDate, dueDate, nullableString(by))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM receivable_invoices WHERE id = $1 AND organization_id = $2)`,
				id, scope.OrganizationID()).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return pgx.ErrNoRows
			}
			return ErrInvoiceNotDraft
		}
		out, err = findInTx(ctx, tx, scope.OrganizationID(), `i.id = $2`, id)
		return err
	})
	return out, err
}

func (r *invoiceRepository) SetPDF(ctx context.Context, scope coretenant.Scope, id, assetID string) error {
	return withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE receivable_invoices SET pdf_asset_id = $3, updated_at = now() WHERE id = $1 AND organization_id = $2`,
			id, scope.OrganizationID(), assetID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *invoiceRepository) Void(ctx context.Context, scope coretenant.Scope, id, reason, by string) (domain.Invoice, error) {
	var out domain.Invoice
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var status string
		var hasPayments bool
		if err := tx.QueryRow(ctx, `SELECT status, amount_paid > 0 FROM receivable_invoices WHERE id = $1 AND organization_id = $2 FOR UPDATE`,
			id, scope.OrganizationID()).Scan(&status, &hasPayments); err != nil {
			return err
		}
		if hasPayments {
			return ErrInvoiceHasPayments
		}
		if status == string(domain.InvoiceVoid) || status == string(domain.InvoicePaid) {
			return ErrInvoiceNotPayable
		}
		if _, err := tx.Exec(ctx, `
			UPDATE receivable_invoices SET status = 'void', voided_at = now(), void_reason = $3, updated_by = $4, updated_at = now()
			WHERE id = $1 AND organization_id = $2`, id, scope.OrganizationID(), reason, nullableString(by)); err != nil {
			return err
		}
		var err error
		out, err = findInTx(ctx, tx, scope.OrganizationID(), `i.id = $2`, id)
		return err
	})
	return out, err
}

func (r *invoiceRepository) MarkOverdue(ctx context.Context, scope coretenant.Scope, today time.Time) (int64, error) {
	var n int64
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE receivable_invoices SET status = 'overdue', updated_at = now()
			WHERE organization_id = $1 AND status = 'issued' AND due_date < $2`, scope.OrganizationID(), today)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		return nil
	})
	return n, err
}
