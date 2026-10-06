package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/shared/pricing"
)

type salesOrderRepository struct{ db *database.Pool }

func NewSalesOrderRepository(db *database.Pool) SalesOrderRepository {
	return &salesOrderRepository{db: db}
}

const salesOrderSelect = `
	SELECT so.id, so.so_number, so.quotation_id, COALESCE(q.quotation_number, ''),
		so.deal_id::text, so.contact_id::text, so.company_id::text, so.status, so.billing_status, so.billing_error,
		so.start_date, so.bill_to_name, so.bill_to_company, so.bill_to_email, so.bill_to_phone, so.bill_to_address,
		so.channels, COALESCE(so.pic_user_id::text, ''), so.currency,
		so.subtotal::text, so.tax_total::text, so.grand_total::text, so.first_invoice_total::text, so.recurring_totals,
		COALESCE(so.receivable_account_id::text, ''), COALESCE(so.initial_invoice_id::text, ''), COALESCE(so.contract_id::text, ''),
		so.confirmed_at, so.cancelled_at, COALESCE(so.confirmed_by::text, ''), COALESCE(so.created_by::text, ''),
		so.created_at, so.updated_at
	FROM crm_sales_orders so
	LEFT JOIN crm_quotations q ON q.organization_id = so.organization_id AND q.id = so.quotation_id`

func scanSalesOrder(row pgx.Row) (domain.SalesOrder, error) {
	var so domain.SalesOrder
	var status, billing string
	var recurring []byte
	err := row.Scan(&so.ID, &so.SONumber, &so.QuotationID, &so.QuotationNumber,
		&so.DealID, &so.ContactID, &so.CompanyID, &status, &billing, &so.BillingError,
		&so.StartDate, &so.BillToName, &so.BillToCompany, &so.BillToEmail, &so.BillToPhone, &so.BillToAddress,
		&so.Channels, &so.PICUserID, &so.Currency,
		&so.Subtotal, &so.TaxTotal, &so.GrandTotal, &so.FirstInvoiceTotal, &recurring,
		&so.ReceivableAccountID, &so.InitialInvoiceID, &so.ContractID,
		&so.ConfirmedAt, &so.CancelledAt, &so.ConfirmedBy, &so.CreatedBy, &so.CreatedAt, &so.UpdatedAt)
	if err != nil {
		return domain.SalesOrder{}, err
	}
	so.Status, so.BillingStatus = domain.SalesOrderStatus(status), domain.BillingStatus(billing)
	so.RecurringTotals = map[pricing.Frequency]string{}
	if len(recurring) > 0 {
		if err := json.Unmarshal(recurring, &so.RecurringTotals); err != nil {
			return domain.SalesOrder{}, err
		}
	}
	return so, nil
}

func loadSalesOrderItems(ctx context.Context, tx pgx.Tx, orgID, soID string) ([]domain.SalesOrderItem, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, description, quantity::text, unit, unit_price::text, COALESCE(discount_percent::text, ''),
			tax_percent::text, tax_amount::text, line_total::text, COALESCE(product_id::text, ''), sku,
			charge_type, COALESCE(billing_frequency, ''), payment_timing, delivery_status, delivered_at,
			COALESCE(delivered_by::text, ''), delivery_note, COALESCE(invoice_id::text, ''), position
		FROM crm_sales_order_items WHERE organization_id = $1 AND sales_order_id = $2
		ORDER BY position, created_at, id`, orgID, soID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.SalesOrderItem{}
	for rows.Next() {
		var it domain.SalesOrderItem
		var charge, freq, timing, delivery string
		if err := rows.Scan(&it.ID, &it.Description, &it.Quantity, &it.Unit, &it.UnitPrice, &it.DiscountPercent,
			&it.TaxPercent, &it.TaxAmount, &it.LineTotal, &it.ProductID, &it.SKU,
			&charge, &freq, &timing, &delivery, &it.DeliveredAt, &it.DeliveredBy, &it.DeliveryNote, &it.InvoiceID, &it.Position); err != nil {
			return nil, err
		}
		it.Pricing = pricing.Attributes{ChargeType: pricing.ChargeType(charge), Frequency: pricing.Frequency(freq), PaymentTiming: pricing.PaymentTiming(timing)}
		it.DeliveryStatus = domain.DeliveryStatus(delivery)
		items = append(items, it)
	}
	return items, rows.Err()
}

func findSalesOrderInTx(ctx context.Context, tx pgx.Tx, orgID, where string, args ...any) (domain.SalesOrder, error) {
	so, err := scanSalesOrder(tx.QueryRow(ctx, salesOrderSelect+` WHERE so.organization_id = $1 AND `+where, append([]any{orgID}, args...)...))
	if err != nil {
		return domain.SalesOrder{}, err
	}
	so.Items, err = loadSalesOrderItems(ctx, tx, orgID, so.ID)
	return so, err
}

func (r *salesOrderRepository) CreateFromQuotation(ctx context.Context, scope coretenant.Scope, q domain.Quotation, number string,
	d SalesOrderDraftFields, createdBy string) (domain.SalesOrder, bool, error) {
	recurring, err := recurringTotalsJSON(q.RecurringTotals)
	if err != nil {
		return domain.SalesOrder{}, false, err
	}
	currency := q.Currency
	if currency == "" {
		currency = "IDR"
	}
	var out domain.SalesOrder
	created := false
	err = withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO crm_sales_orders (organization_id, so_number, quotation_id, deal_id, contact_id, company_id,
				start_date, bill_to_name, bill_to_company, bill_to_email, bill_to_phone, bill_to_address, channels, pic_user_id,
				currency, subtotal, tax_total, grand_total, first_invoice_total, recurring_totals, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
			ON CONFLICT (organization_id, quotation_id) DO NOTHING
			RETURNING id`,
			scope.OrganizationID(), number, q.ID, q.DealID, q.ContactID, q.CompanyID,
			d.StartDate, d.BillToName, d.BillToCompany, d.BillToEmail, d.BillToPhone, d.BillToAddress, channelsOrEmpty(d.Channels),
			nullableString(d.PICUserID), currency, totalOrZero(q.Subtotal), totalOrZero(q.TaxTotal), totalOrZero(q.GrandTotal),
			totalOrZero(q.FirstInvoiceTotal), recurring, nullableString(createdBy)).Scan(&id)
		if err == pgx.ErrNoRows { // quotation ini sudah punya SO
			out, err = findSalesOrderInTx(ctx, tx, scope.OrganizationID(), `so.quotation_id = $2`, q.ID)
			return err
		}
		if err != nil {
			return err
		}
		created = true
		for i, it := range q.Items {
			attrs := it.Pricing
			if attrs.ChargeType == "" {
				attrs.ChargeType = pricing.OneTime
			}
			if attrs.PaymentTiming == "" {
				attrs.PaymentTiming = pricing.Prepaid
			}
			delivery := domain.DeliveryNotApplicable
			if attrs.ChargeType == pricing.OneTime && attrs.PaymentTiming == pricing.Postpaid {
				delivery = domain.DeliveryPending
			}
			var discount any
			if it.DiscountPercent != nil && *it.DiscountPercent != "" {
				discount = *it.DiscountPercent
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO crm_sales_order_items (organization_id, sales_order_id, description, quantity, unit, unit_price,
					discount_percent, tax_percent, tax_amount, line_total, product_id, sku, charge_type, billing_frequency,
					payment_timing, delivery_status, position)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
				scope.OrganizationID(), id, it.Description, it.Quantity, it.Unit, it.UnitPrice, discount,
				totalOrZero(it.TaxPercent), totalOrZero(it.TaxAmount), it.LineTotal, it.ProductID, it.SKU,
				string(attrs.ChargeType), nullableString(string(attrs.Frequency)), string(attrs.PaymentTiming),
				string(delivery), i); err != nil {
				return err
			}
		}
		out, err = findSalesOrderInTx(ctx, tx, scope.OrganizationID(), `so.id = $2`, id)
		return err
	})
	return out, created, err
}

func channelsOrEmpty(c []string) []string {
	if c == nil {
		return []string{}
	}
	return c
}

func (r *salesOrderRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.SalesOrder, error) {
	var out domain.SalesOrder
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = findSalesOrderInTx(ctx, tx, scope.OrganizationID(), `so.id = $2`, id)
		return err
	})
	return out, err
}

func (r *salesOrderRepository) FindByQuotation(ctx context.Context, scope coretenant.Scope, quotationID string) (domain.SalesOrder, error) {
	var out domain.SalesOrder
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = findSalesOrderInTx(ctx, tx, scope.OrganizationID(), `so.quotation_id = $2`, quotationID)
		return err
	})
	return out, err
}

func (r *salesOrderRepository) ListByDeal(ctx context.Context, scope coretenant.Scope, dealID string) ([]domain.SalesOrder, error) {
	out := []domain.SalesOrder{}
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, salesOrderSelect+` WHERE so.organization_id = $1 AND so.deal_id = $2 ORDER BY so.created_at DESC, so.id`,
			scope.OrganizationID(), dealID)
		if err != nil {
			return err
		}
		for rows.Next() {
			so, err := scanSalesOrder(rows)
			if err != nil {
				rows.Close()
				return err
			}
			out = append(out, so)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range out {
			if out[i].Items, err = loadSalesOrderItems(ctx, tx, scope.OrganizationID(), out[i].ID); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

func (r *salesOrderRepository) List(ctx context.Context, scope coretenant.Scope, f SalesOrderListFilter) ([]domain.SalesOrder, int64, error) {
	conds := []string{"so.organization_id = $1"}
	args := []any{scope.OrganizationID()}
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}
	if f.Status != "" {
		add("so.status = ?", f.Status)
	}
	if f.BillingStatus != "" {
		add("so.billing_status = ?", f.BillingStatus)
	}
	if f.DealID != "" {
		add("so.deal_id = ?", f.DealID)
	}
	if f.CompanyID != "" {
		add("so.company_id = ?", f.CompanyID)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		add("(so.so_number ILIKE ? OR so.bill_to_name ILIKE ? OR so.bill_to_company ILIKE ? OR q.quotation_number ILIKE ?)", "%"+escapeLike(s)+"%")
	}
	where := strings.Join(conds, " AND ")
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out := []domain.SalesOrder{}
	var total int64
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM crm_sales_orders so
			LEFT JOIN crm_quotations q ON q.organization_id = so.organization_id AND q.id = so.quotation_id
			WHERE `+where, args...).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, salesOrderSelect+` WHERE `+where+fmt.Sprintf(` ORDER BY so.created_at DESC, so.id LIMIT %d OFFSET %d`, limit, max(f.Offset, 0)), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			so, err := scanSalesOrder(rows)
			if err != nil {
				return err
			}
			out = append(out, so)
		}
		return rows.Err()
	})
	return out, total, err
}

// escapeLike meloloskan metakarakter LIKE pada input pencarian.
func escapeLike(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\\' || r == '%' || r == '_' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}

// conditional menjalankan UPDATE dan memetakan nol baris: SO tidak ada → pgx.ErrNoRows, ada tapi syarat gagal → notMatch.
func conditionalUpdate(ctx context.Context, tx pgx.Tx, orgID, id string, notMatch error, sql string, args ...any) error {
	tag, err := tx.Exec(ctx, sql, append([]any{id, orgID}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM crm_sales_orders WHERE id = $1 AND organization_id = $2)`, id, orgID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return pgx.ErrNoRows
	}
	return notMatch
}

func (r *salesOrderRepository) mutate(ctx context.Context, scope coretenant.Scope, id string, fn func(tx pgx.Tx) error) (domain.SalesOrder, error) {
	var out domain.SalesOrder
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if err := fn(tx); err != nil {
			return err
		}
		var err error
		out, err = findSalesOrderInTx(ctx, tx, scope.OrganizationID(), `so.id = $2`, id)
		return err
	})
	return out, err
}

func (r *salesOrderRepository) UpdateDraft(ctx context.Context, scope coretenant.Scope, id string, f SalesOrderDraftFields, by string) (domain.SalesOrder, error) {
	return r.mutate(ctx, scope, id, func(tx pgx.Tx) error {
		return conditionalUpdate(ctx, tx, scope.OrganizationID(), id, ErrSalesOrderNotDraft, `
			UPDATE crm_sales_orders SET start_date = $3, bill_to_name = $4, bill_to_company = $5, bill_to_email = $6,
				bill_to_phone = $7, bill_to_address = $8, channels = $9, pic_user_id = $10, updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND status = 'draft'`,
			f.StartDate, f.BillToName, f.BillToCompany, f.BillToEmail, f.BillToPhone, f.BillToAddress,
			channelsOrEmpty(f.Channels), nullableString(f.PICUserID))
	})
}

func (r *salesOrderRepository) MarkConfirmed(ctx context.Context, scope coretenant.Scope, id, by string) (domain.SalesOrder, error) {
	return r.mutate(ctx, scope, id, func(tx pgx.Tx) error {
		return conditionalUpdate(ctx, tx, scope.OrganizationID(), id, ErrSalesOrderNotDraft, `
			UPDATE crm_sales_orders SET status = 'confirmed', billing_status = 'pending', billing_error = '',
				confirmed_at = now(), confirmed_by = $3, updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND status = 'draft'`, nullableString(by))
	})
}

// completeIfSettled: confirmed → completed bila tak ada item menunggu diterima dan setiap baris one_time prabayar
// sudah ditagih (initial_invoice_id terisi).
const completeIfSettled = `
	UPDATE crm_sales_orders so SET status = 'completed', updated_at = now()
	WHERE so.id = $1 AND so.organization_id = $2 AND so.status = 'confirmed'
		AND NOT EXISTS (SELECT 1 FROM crm_sales_order_items i WHERE i.organization_id = so.organization_id
			AND i.sales_order_id = so.id AND i.delivery_status = 'pending')
		AND (so.initial_invoice_id IS NOT NULL OR NOT EXISTS (SELECT 1 FROM crm_sales_order_items i
			WHERE i.organization_id = so.organization_id AND i.sales_order_id = so.id
				AND i.charge_type = 'one_time' AND i.payment_timing = 'prepaid'))`

func (r *salesOrderRepository) SetBillingResult(ctx context.Context, scope coretenant.Scope, id string, status domain.BillingStatus,
	errMsg, accountID, initialInvoiceID, contractID string) (domain.SalesOrder, error) {
	return r.mutate(ctx, scope, id, func(tx pgx.Tx) error {
		if err := conditionalUpdate(ctx, tx, scope.OrganizationID(), id, ErrSalesOrderNotDraft, `
			UPDATE crm_sales_orders SET billing_status = $3, billing_error = $4,
				receivable_account_id = COALESCE($5::uuid, receivable_account_id),
				initial_invoice_id = COALESCE($6::uuid, initial_invoice_id),
				contract_id = COALESCE($7::uuid, contract_id), updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND status IN ('confirmed', 'completed')`,
			string(status), errMsg, nullableString(accountID), nullableString(initialInvoiceID), nullableString(contractID)); err != nil {
			return err
		}
		if status == domain.BillingDone {
			_, err := tx.Exec(ctx, completeIfSettled, id, scope.OrganizationID())
			return err
		}
		return nil
	})
}

func (r *salesOrderRepository) MarkDelivered(ctx context.Context, scope coretenant.Scope, id string, itemIDs []string, at time.Time,
	note, invoiceID, by string) (domain.SalesOrder, error) {
	return r.mutate(ctx, scope, id, func(tx pgx.Tx) error {
		if len(itemIDs) == 0 {
			return ErrDeliveryNotPending
		}
		// Kunci SO supaya konfirmasi paralel untuk item yang sama berurutan.
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM crm_sales_orders WHERE id = $1 AND organization_id = $2 FOR UPDATE`,
			id, scope.OrganizationID()).Scan(&status); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE crm_sales_order_items SET delivery_status = 'delivered', delivered_at = $4, delivered_by = $5,
				delivery_note = $6, invoice_id = $7
			WHERE organization_id = $1 AND sales_order_id = $2 AND id = ANY($3::uuid[]) AND delivery_status = 'pending'`,
			scope.OrganizationID(), id, itemIDs, at, nullableString(by), note, nullableString(invoiceID))
		if err != nil {
			return err
		}
		if int(tag.RowsAffected()) != len(itemIDs) {
			return ErrDeliveryNotPending // rollback: tidak ada yang berubah
		}
		_, err = tx.Exec(ctx, completeIfSettled, id, scope.OrganizationID())
		return err
	})
}

func (r *salesOrderRepository) Cancel(ctx context.Context, scope coretenant.Scope, id, by string) (domain.SalesOrder, error) {
	return r.mutate(ctx, scope, id, func(tx pgx.Tx) error {
		return conditionalUpdate(ctx, tx, scope.OrganizationID(), id, ErrSalesOrderNotDraft, `
			UPDATE crm_sales_orders SET status = 'cancelled', cancelled_at = now(), updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND status = 'draft'`)
	})
}

// MarkCancelled: confirmed → cancelled (SO yang belum dibayar; pemanggil sudah memeriksa invoice).
func (r *salesOrderRepository) MarkCancelled(ctx context.Context, scope coretenant.Scope, id, by string) (domain.SalesOrder, error) {
	return r.mutate(ctx, scope, id, func(tx pgx.Tx) error {
		return conditionalUpdate(ctx, tx, scope.OrganizationID(), id, ErrSalesOrderNotConfirmed, `
			UPDATE crm_sales_orders SET status = 'cancelled', cancelled_at = now(), updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND status = 'confirmed'`)
	})
}
