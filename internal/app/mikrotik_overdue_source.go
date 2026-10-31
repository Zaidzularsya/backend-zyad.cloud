package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
)

// triggerInvoiceCond selects candidate invoices (alias i): issued/overdue, due
// strictly before $2 (today), and holding an item whose product has an enabled
// trigger with effective_from <= due_date. Partial payment does not matter; the
// invoice leaves the set only when it becomes paid/void. $1 = organization id.
const triggerInvoiceCond = `
	i.organization_id = $1
	AND i.status IN ('issued', 'overdue')
	AND i.due_date < $2::date
	AND EXISTS (
		SELECT 1
		FROM receivable_invoice_items ii
		JOIN mikrotik_product_triggers t
		  ON t.organization_id = ii.organization_id AND t.product_id = ii.product_id
		WHERE ii.organization_id = i.organization_id AND ii.invoice_id = i.id
		  AND ii.product_id IS NOT NULL
		  AND t.enabled AND i.due_date >= t.effective_from
	)`

var _ mikrotikservice.OverdueSource = mikrotikReceivableReader{}

// TriggerInvoices lists the account's candidate invoices ordered by due date.
func (r mikrotikReceivableReader) TriggerInvoices(ctx context.Context, scope coretenant.Scope, accountID string, today time.Time) ([]mikrotikservice.TriggerInvoice, error) {
	out := []mikrotikservice.TriggerInvoice{}
	err := r.scopedRead(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT i.id::text, COALESCE(i.invoice_number, ''), i.due_date, (i.grand_total - i.amount_paid)::text
			FROM receivable_invoices i
			WHERE `+triggerInvoiceCond+` AND i.account_id = $3::uuid
			ORDER BY i.due_date, i.id`,
			scope.OrganizationID(), today, accountID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var inv mikrotikservice.TriggerInvoice
			if err := rows.Scan(&inv.ID, &inv.Number, &inv.DueDate, &inv.Balance); err != nil {
				return err
			}
			out = append(out, inv)
		}
		return rows.Err()
	})
	return out, err
}

// AccountsWithTriggerInvoices returns up to limit account ids (ascending, after
// afterAccountID) that own at least one candidate invoice.
func (r mikrotikReceivableReader) AccountsWithTriggerInvoices(ctx context.Context, scope coretenant.Scope, today time.Time, afterAccountID string, limit int) ([]string, error) {
	out := []string{}
	if limit <= 0 {
		return out, nil
	}
	err := r.scopedRead(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT i.account_id::text
			FROM receivable_invoices i
			WHERE `+triggerInvoiceCond+` AND ($3 = '' OR i.account_id > $3::uuid)
			ORDER BY 1
			LIMIT $4`,
			scope.OrganizationID(), today, afterAccountID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out = append(out, id)
		}
		return rows.Err()
	})
	return out, err
}

// AccountOfInvoice returns the invoice's account id, or mikrotik.ErrAccountNotFound.
func (r mikrotikReceivableReader) AccountOfInvoice(ctx context.Context, scope coretenant.Scope, invoiceID string) (string, error) {
	var id string
	err := r.scopedRead(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT account_id::text FROM receivable_invoices WHERE organization_id = $1 AND id = $2::uuid`,
			scope.OrganizationID(), invoiceID).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", mikrotik.ErrAccountNotFound
	}
	return id, err
}
