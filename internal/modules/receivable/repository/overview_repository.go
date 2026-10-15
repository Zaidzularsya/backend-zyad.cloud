package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/shared/pricing"
)

type overviewRepository struct{ db *database.Pool }

func NewOverviewRepository(db *database.Pool) OverviewRepository { return &overviewRepository{db: db} }

func (r *overviewRepository) ActiveItems(ctx context.Context, scope coretenant.Scope, limit int) ([]ActiveItem, error) {
	out := []ActiveItem{}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT c.id, c.contract_number, COALESCE(NULLIF(a.company_name, ''), a.name), c.start_date, c.end_date,
				i.id, i.description, i.quantity::text, i.unit, i.unit_price::text, COALESCE(i.discount_percent::text, ''),
				i.tax_percent::text, i.billing_frequency, i.payment_timing, i.period_index, i.next_period_start, i.next_period_end, i.position
			FROM receivable_contracts c
			JOIN receivable_accounts a ON a.organization_id = c.organization_id AND a.id = c.account_id
			JOIN receivable_contract_items i ON i.organization_id = c.organization_id AND i.contract_id = c.id
			WHERE c.organization_id = $1 AND c.status = 'active'
			ORDER BY c.id, i.position, i.id
			LIMIT $2`, scope.OrganizationID(), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a ActiveItem
			var freq, timing string
			it := &a.Item
			if err := rows.Scan(&a.ContractID, &a.ContractNumber, &a.AccountName, &a.StartDate, &a.EndDate,
				&it.ID, &it.Description, &it.Quantity, &it.Unit, &it.UnitPrice, &it.DiscountPercent,
				&it.TaxPercent, &freq, &timing, &it.PeriodIndex, &it.NextPeriodStart, &it.NextPeriodEnd, &it.Position); err != nil {
				return err
			}
			it.Frequency, it.PaymentTiming = pricing.Frequency(freq), pricing.PaymentTiming(timing)
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

func (r *overviewRepository) Unpaid(ctx context.Context, scope coretenant.Scope, today time.Time) (UnpaidSummary, error) {
	var out UnpaidSummary
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*), count(*) FILTER (WHERE status = 'overdue' OR due_date < $2::date),
				COALESCE(sum(grand_total - amount_paid), 0)::numeric(18,2)::text
			FROM receivable_invoices
			WHERE organization_id = $1 AND status IN ('issued', 'overdue')`, scope.OrganizationID(), today).
			Scan(&out.Count, &out.OverdueCount, &out.TotalBalance)
	})
	return out, err
}

func (r *overviewRepository) FailedSends(ctx context.Context, scope coretenant.Scope, limit int) ([]FailedSend, error) {
	out := []FailedSend{}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT * FROM (
				SELECT DISTINCT ON (s.invoice_id, s.channel) s.invoice_id, COALESCE(i.invoice_number, ''),
					COALESCE(NULLIF(a.company_name, ''), a.name), s.channel, s.error, s.sent_at
				FROM receivable_invoice_sends s
				JOIN receivable_invoices i ON i.organization_id = s.organization_id AND i.id = s.invoice_id
				JOIN receivable_accounts a ON a.organization_id = i.organization_id AND a.id = i.account_id
				WHERE s.organization_id = $1 AND s.status = 'failed'
					AND NOT EXISTS (
						SELECT 1 FROM receivable_invoice_sends ok
						WHERE ok.organization_id = s.organization_id AND ok.invoice_id = s.invoice_id
							AND ok.channel = s.channel AND ok.status = 'sent' AND ok.sent_at >= s.sent_at)
				ORDER BY s.invoice_id, s.channel, s.sent_at DESC, s.id DESC
			) latest ORDER BY sent_at DESC LIMIT $2`, scope.OrganizationID(), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var f FailedSend
			if err := rows.Scan(&f.InvoiceID, &f.InvoiceNumber, &f.AccountName, &f.Channel, &f.Error, &f.SentAt); err != nil {
				return err
			}
			out = append(out, f)
		}
		return rows.Err()
	})
	return out, err
}
