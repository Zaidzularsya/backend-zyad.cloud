package app

import (
	"context"
	"errors"
	"strings"
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

// ---- CandidateSource: the isolation preview read model ----

var _ mikrotikservice.CandidateSource = mikrotikReceivableReader{}

// candidateCTE builds "cr": one row per account with >=1 candidate invoice
// ($1 org, $2 cutoff, $3 today, $4 uuid[] account filter or NULL) with its
// preview status. Secrets count only on non-deleted routers. The CASE mirrors
// service.CandidateStatusOf (kept in sync by TestCandidateStatus_SQLMatchesGo)
// and predicts the engine: only normal, present secrets with a known original
// profile on an isolir-ready router with an isolir profile name are cut.
const candidateCTE = `
	WITH cand AS (
		SELECT i.id, i.account_id, i.invoice_number, i.due_date, i.grand_total - i.amount_paid AS balance
		FROM receivable_invoices i
		WHERE ` + triggerInvoiceCond + `
		  AND ($4::uuid[] IS NULL OR i.account_id = ANY($4::uuid[]))
	), acc AS (
		SELECT account_id, min(due_date) AS oldest_due, count(*) AS invoice_count, sum(balance) AS balance,
		       (array_agg(invoice_number ORDER BY due_date, id))[1] AS oldest_number
		FROM cand
		GROUP BY account_id
	), sec AS (
		SELECT s.account_id,
		       bool_and(s.isolation_state = 'isolated') AS all_isolated,
		       bool_or(s.isolation_state = 'normal' AND s.present AND NOT s.needs_original_profile
		               AND r.isolir_ready AND COALESCE(r.isolir_profile_name, '') <> '') AS any_actionable,
		       bool_or(s.isolation_state = 'normal' AND s.needs_original_profile) AS needs_profile,
		       bool_or(s.isolation_state = 'normal' AND s.present) AS any_normal_present,
		       (array_agg(r.name ORDER BY s.name, s.id))[1] AS router_name
		FROM mikrotik_ppp_secrets s
		JOIN mikrotik_routers r ON r.organization_id = s.organization_id AND r.id = s.router_id AND r.deleted_at IS NULL
		WHERE s.organization_id = $1 AND s.account_id IN (SELECT account_id FROM acc)
		GROUP BY s.account_id
	), cr AS (
		SELECT a.account_id, a.oldest_due, a.invoice_count, a.balance, a.oldest_number,
		       ra.name, ra.phone, sec.router_name,
		       CASE WHEN p.exempt THEN p.exempted_until END AS exempted_until,
		       pa.account_id IS NOT NULL AS pilot,
		       CASE
		           WHEN COALESCE(p.exempt, false) AND (p.exempted_until IS NULL OR p.exempted_until >= $3::date) THEN 'exempt'
		           WHEN sec.account_id IS NULL THEN 'no_secret'
		           WHEN sec.all_isolated THEN 'already_isolated'
		           WHEN sec.any_actionable THEN 'will_isolate'
		           WHEN sec.needs_profile THEN 'needs_profile'
		           WHEN NOT sec.any_normal_present THEN 'secret_missing'
		           ELSE 'router_not_ready'
		       END AS status
		FROM acc a
		JOIN receivable_accounts ra ON ra.organization_id = $1 AND ra.id = a.account_id
		LEFT JOIN sec ON sec.account_id = a.account_id
		LEFT JOIN mikrotik_customer_policies p ON p.organization_id = $1 AND p.account_id = a.account_id
		LEFT JOIN mikrotik_pilot_accounts pa ON pa.organization_id = $1 AND pa.account_id = a.account_id
	)`

// candidateWhere applies CandidateFilter.Status ($5, "" = all) and Q ($6, an
// escaped ILIKE pattern, "" = all).
const candidateWhere = `
	WHERE ($5 = '' OR cr.status = $5)
	  AND ($6 = '' OR cr.name ILIKE $6 ESCAPE '\' OR cr.phone ILIKE $6 ESCAPE '\')`

// candidateProducts lists up to $9 trigger product names of the account's
// candidate invoices (same triggerInvoiceCond), read only for the page rows.
const candidateProducts = `ARRAY(
	SELECT x.n FROM (
		SELECT DISTINCT COALESCE(cp.name, ii.description) AS n
		FROM receivable_invoices i
		JOIN receivable_invoice_items ii ON ii.organization_id = i.organization_id AND ii.invoice_id = i.id
		JOIN mikrotik_product_triggers t
		  ON t.organization_id = ii.organization_id AND t.product_id = ii.product_id
		 AND t.enabled AND i.due_date >= t.effective_from
		LEFT JOIN catalog_products cp ON cp.organization_id = ii.organization_id AND cp.id = ii.product_id
		WHERE ` + triggerInvoiceCond + ` AND i.account_id = cr.account_id
	) x ORDER BY x.n LIMIT $9)`

// maxCandidateProducts is how many product names a row lists before "…".
const maxCandidateProducts = 5

// likePattern escapes LIKE wildcards and wraps q as a substring pattern.
func likePattern(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

// candidateAccountIDs is NULL (no filter) for a nil slice.
func candidateAccountIDs(ids []string) any {
	if ids == nil {
		return nil
	}
	return ids
}

// candidatePageSQL is the page query. Offset pages carry the total as
// count(*) OVER () (same statement and snapshot as the rows); keyset pages
// ($10/$11 = cursor) skip both the count and the offset. $9 is the product
// name cap; status-only reads replace the product subquery with an empty array.
func candidatePageSQL(keyset, statusOnly bool) string {
	total, keyCond, products := `count(*) OVER ()`, ``, candidateProducts
	if keyset {
		total = `0::bigint`
		keyCond = ` AND (cr.oldest_due, cr.account_id) > ($10::date, $11::uuid)`
	}
	if statusOnly {
		products = `CASE WHEN $9::int > 0 THEN '{}'::text[] END`
	}
	return candidateCTE + `
		SELECT cr.account_id::text, cr.name, cr.phone, cr.status, COALESCE(cr.router_name, ''),
		       cr.oldest_due, ($3::date - cr.oldest_due), cr.invoice_count, COALESCE(cr.oldest_number, ''),
		       cr.balance::text, cr.exempted_until, cr.pilot, ` + products + `, ` + total + `
		FROM cr` + candidateWhere + keyCond + `
		ORDER BY cr.oldest_due, cr.account_id
		LIMIT $7 OFFSET $8`
}

func (r mikrotikReceivableReader) ListCandidates(ctx context.Context, scope coretenant.Scope, today, cutoff time.Time, f mikrotikservice.CandidateFilter, limit, offset int) ([]mikrotikservice.CandidateRow, int64, error) {
	out := []mikrotikservice.CandidateRow{}
	var total int64
	if limit <= 0 {
		return out, 0, nil
	}
	if offset < 0 || f.After != nil {
		offset = 0
	}
	args := []any{scope.OrganizationID(), cutoff, today, candidateAccountIDs(f.AccountIDs), f.Status, likePattern(f.Q), limit, offset, maxCandidateProducts + 1}
	if f.After != nil {
		args = append(args, f.After.OldestDue, f.After.AccountID)
	}
	err := r.scopedRead(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, candidatePageSQL(f.After != nil, f.StatusOnly), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row mikrotikservice.CandidateRow
			var products []string
			if err := rows.Scan(&row.AccountID, &row.AccountName, &row.Phone, &row.Status, &row.RouterName,
				&row.OldestDue, &row.OverdueDays, &row.InvoiceCount, &row.OldestInvoiceNumber, &row.Balance,
				&row.ExemptUntil, &row.Pilot, &products, &total); err != nil {
				return err
			}
			if len(products) > maxCandidateProducts {
				products = append(products[:maxCandidateProducts], "…")
			}
			row.Products = strings.Join(products, ", ")
			out = append(out, row)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out) == 0 && offset > 0 && f.After == nil {
			// Past the last page there is no row to carry the window total.
			return tx.QueryRow(ctx, candidateCTE+` SELECT count(*) FROM cr`+candidateWhere, args[:6]...).Scan(&total)
		}
		return nil
	})
	return out, total, err
}

// Summary counts the same "cr" rows as ListCandidates in one grouped query,
// so the list and the summary cannot disagree.
func (r mikrotikReceivableReader) Summary(ctx context.Context, scope coretenant.Scope, today, cutoff time.Time) (mikrotikservice.CandidateSummary, error) {
	var s mikrotikservice.CandidateSummary
	err := r.scopedRead(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, candidateCTE+`
			SELECT count(*) FILTER (WHERE status = 'will_isolate'),
			       count(*) FILTER (WHERE status = 'exempt'),
			       count(*) FILTER (WHERE status IN ('no_secret', 'needs_profile', 'secret_missing', 'router_not_ready')),
			       count(*) FILTER (WHERE status = 'already_isolated'),
			       (SELECT count(DISTINCT s.account_id)
			          FROM mikrotik_ppp_secrets s
			          JOIN mikrotik_routers r ON r.organization_id = s.organization_id AND r.id = s.router_id AND r.deleted_at IS NULL
			         WHERE s.organization_id = $1 AND s.account_id IS NOT NULL)
			FROM cr`,
			scope.OrganizationID(), cutoff, today, nil).
			Scan(&s.Target, &s.Exempt, &s.Blocked, &s.AlreadyIsolated, &s.PairedAccounts)
	})
	return s, err
}
