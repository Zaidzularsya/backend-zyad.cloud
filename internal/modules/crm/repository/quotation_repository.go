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

type quotationRepository struct {
	db *database.Pool
}

func NewQuotationRepository(db *database.Pool) QuotationRepository {
	return &quotationRepository{db: db}
}

func (r *quotationRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID())
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

const quotationColumns = `
	id, organization_id, deal_id, contact_id, company_id, quotation_number, status, valid_until,
	subtotal::text, discount_total::text, tax_total::text, grand_total::text, currency, notes,
	sent_at, approved_at, rejected_at, created_by, updated_by, created_at, updated_at, deleted_at,
	revision_of_id, revision_no, pdf_asset_id, pdf_generated_at,
	one_time_total::text, first_invoice_total::text, recurring_totals, channel
`

const quotationItemColumns = `
	id, description, quantity::text, unit_price::text, discount_percent::text, line_total::text, position,
	product_id, sku, unit, tax_percent::text, tax_amount::text,
	charge_type, billing_frequency, payment_timing, features
`

func scanQuotation(row pgx.Row) (domain.Quotation, error) {
	var q domain.Quotation
	var dealID, contactID, companyID *string
	var notes *string
	var createdBy, updatedBy *string
	var status string
	var recurringJSON []byte
	var channel *string

	err := row.Scan(
		&q.ID, &q.OrganizationID, &dealID, &contactID, &companyID, &q.QuotationNumber, &status, &q.ValidUntil,
		&q.Subtotal, &q.DiscountTotal, &q.TaxTotal, &q.GrandTotal, &q.Currency, &notes,
		&q.SentAt, &q.ApprovedAt, &q.RejectedAt, &createdBy, &updatedBy, &q.CreatedAt, &q.UpdatedAt, &q.DeletedAt,
		&q.RevisionOfID, &q.RevisionNo, &q.PDFAssetID, &q.PDFGeneratedAt,
		&q.OneTimeTotal, &q.FirstInvoiceTotal, &recurringJSON, &channel,
	)
	if err != nil {
		return domain.Quotation{}, err
	}
	q.RecurringTotals = map[pricing.Frequency]string{}
	if len(recurringJSON) > 0 {
		if err := json.Unmarshal(recurringJSON, &q.RecurringTotals); err != nil {
			return domain.Quotation{}, err
		}
	}

	if channel != nil {
		q.Channel = *channel
	}
	q.Status = domain.QuotationStatus(status)
	q.DealID = dealID
	q.ContactID = contactID
	q.CompanyID = companyID
	if notes != nil {
		q.Notes = *notes
	}
	if createdBy != nil {
		q.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		q.UpdatedBy = *updatedBy
	}

	return q, nil
}

func scanQuotationItem(row pgx.Row) (domain.QuotationItem, error) {
	var item domain.QuotationItem
	var sku, unit, frequency *string
	var featuresJSON []byte
	err := row.Scan(&item.ID, &item.Description, &item.Quantity, &item.UnitPrice, &item.DiscountPercent, &item.LineTotal, &item.Position,
		&item.ProductID, &sku, &unit, &item.TaxPercent, &item.TaxAmount,
		&item.Pricing.ChargeType, &frequency, &item.Pricing.PaymentTiming, &featuresJSON)
	if err != nil {
		return item, err
	}
	item.Features = []domain.FeatureSnapshot{}
	if len(featuresJSON) > 0 {
		if err := json.Unmarshal(featuresJSON, &item.Features); err != nil {
			return item, err
		}
	}
	if frequency != nil {
		item.Pricing.Frequency = pricing.Frequency(*frequency)
	}
	if sku != nil {
		item.SKU = *sku
	}
	if unit != nil {
		item.Unit = *unit
	}
	return item, nil
}

func insertQuotationItemsTx(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, quotationID string, items []QuotationItemInput) error {
	for _, item := range items {
		taxPercent := item.TaxPercent
		if taxPercent == "" {
			taxPercent = "0"
		}
		attrs := item.Pricing
		if attrs.ChargeType == "" {
			attrs.ChargeType = pricing.OneTime
		}
		if attrs.PaymentTiming == "" {
			attrs.PaymentTiming = pricing.Prepaid
		}
		taxAmount := item.TaxAmount
		if taxAmount == "" {
			taxAmount = "0"
		}
		features := item.Features
		if features == nil {
			features = []domain.FeatureSnapshot{}
		}
		featuresJSON, err := json.Marshal(features)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO crm_quotation_items (
				organization_id, quotation_id, description, quantity, unit_price, discount_percent, line_total, position,
				product_id, sku, unit, tax_percent, tax_amount, charge_type, billing_frequency, payment_timing, features
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
			scope.OrganizationID(), quotationID, item.Description, item.Quantity, item.UnitPrice,
			nullableString(item.DiscountPercent), item.LineTotal, item.Position,
			nullableString(item.ProductID), nullableString(item.SKU), nullableString(item.Unit), taxPercent, taxAmount,
			attrs.ChargeType, nullableString(string(attrs.Frequency)), attrs.PaymentTiming, featuresJSON); err != nil {
			return err
		}
	}
	return nil
}

func loadQuotationItems(ctx context.Context, tx pgx.Tx, organizationID string, quotationID string) ([]domain.QuotationItem, error) {
	query := `SELECT ` + quotationItemColumns + ` FROM crm_quotation_items
		WHERE organization_id = $1 AND quotation_id = $2
		ORDER BY position ASC`

	rows, err := tx.Query(ctx, query, organizationID, quotationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.QuotationItem, 0)
	for rows.Next() {
		item, err := scanQuotationItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// recurringTotalsJSON menyerialkan rincian berulang; nil → objek kosong agar
// kolom jsonb NOT NULL tidak pernah menerima null.
func recurringTotalsJSON(m map[pricing.Frequency]string) ([]byte, error) {
	if m == nil {
		m = map[pricing.Frequency]string{}
	}
	return json.Marshal(m)
}

func totalOrZero(v string) string {
	if v == "" {
		return "0"
	}
	return v
}

func (r *quotationRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateQuotationParams) (domain.Quotation, error) {
	if !scope.IsValid() {
		return domain.Quotation{}, coretenant.ErrInvalidScope
	}

	currency := params.Currency
	if currency == "" {
		currency = "IDR"
	}

	query := `
		INSERT INTO crm_quotations (
			organization_id, deal_id, contact_id, company_id, quotation_number, valid_until,
			subtotal, discount_total, tax_total, grand_total, currency, notes, created_by,
			revision_of_id, revision_no, one_time_total, first_invoice_total, recurring_totals, channel
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
		) RETURNING ` + quotationColumns

	recurringJSON, err := recurringTotalsJSON(params.RecurringTotals)
	if err != nil {
		return domain.Quotation{}, err
	}
	var quotation domain.Quotation
	err = r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		quotation, scanErr = scanQuotation(tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			nullableString(params.DealID),
			nullableString(params.ContactID),
			nullableString(params.CompanyID),
			params.QuotationNumber,
			params.ValidUntil,
			params.Subtotal,
			params.DiscountTotal,
			params.TaxTotal,
			params.GrandTotal,
			currency,
			nullableString(params.Notes),
			nullableString(params.CreatedBy),
			nullableString(params.RevisionOfID),
			params.RevisionNo,
			totalOrZero(params.OneTimeTotal),
			totalOrZero(params.FirstInvoiceTotal),
			recurringJSON,
			nullableString(params.Channel),
		))
		if scanErr != nil {
			return scanErr
		}

		if err := insertQuotationItemsTx(ctx, tx, scope, quotation.ID, params.Items); err != nil {
			return err
		}

		items, err := loadQuotationItems(ctx, tx, scope.OrganizationID(), quotation.ID)
		if err != nil {
			return err
		}
		quotation.Items = items
		return nil
	})
	if err != nil {
		return domain.Quotation{}, err
	}
	return quotation, nil
}

func (r *quotationRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Quotation, error) {
	if !scope.IsValid() {
		return domain.Quotation{}, coretenant.ErrInvalidScope
	}

	query := `SELECT ` + quotationColumns + ` FROM crm_quotations WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	var quotation domain.Quotation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		quotation, scanErr = scanQuotation(tx.QueryRow(ctx, query, id, scope.OrganizationID()))
		if scanErr != nil {
			return scanErr
		}
		items, err := loadQuotationItems(ctx, tx, scope.OrganizationID(), quotation.ID)
		if err != nil {
			return err
		}
		quotation.Items = items
		return nil
	})
	if err != nil {
		return domain.Quotation{}, err
	}
	return quotation, nil
}

func (r *quotationRepository) List(ctx context.Context, scope coretenant.Scope, filter QuotationListFilter) ([]domain.Quotation, int64, error) {
	if !scope.IsValid() {
		return nil, 0, coretenant.ErrInvalidScope
	}

	whereClauses := []string{"organization_id = $1"}
	args := []interface{}{scope.OrganizationID()}

	if !filter.IncludeDeleted {
		whereClauses = append(whereClauses, "deleted_at IS NULL")
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.DealID != "" {
		args = append(args, filter.DealID)
		whereClauses = append(whereClauses, fmt.Sprintf("deal_id = $%d", len(args)))
	}
	if filter.ContactID != "" {
		args = append(args, filter.ContactID)
		whereClauses = append(whereClauses, fmt.Sprintf("contact_id = $%d", len(args)))
	}
	if filter.CompanyID != "" {
		args = append(args, filter.CompanyID)
		whereClauses = append(whereClauses, fmt.Sprintf("company_id = $%d", len(args)))
	}

	where := strings.Join(whereClauses, " AND ")
	countQuery := "SELECT COUNT(*) FROM crm_quotations WHERE " + where

	var quotations []domain.Quotation
	var total int64

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			quotations = []domain.Quotation{}
			return nil
		}

		query := "SELECT " + quotationColumns + " FROM crm_quotations WHERE " + where + " ORDER BY created_at DESC"
		queryArgs := append([]interface{}{}, args...)
		if filter.Limit > 0 {
			queryArgs = append(queryArgs, filter.Limit)
			query += fmt.Sprintf(" LIMIT $%d", len(queryArgs))
		}
		if filter.Offset > 0 {
			queryArgs = append(queryArgs, filter.Offset)
			query += fmt.Sprintf(" OFFSET $%d", len(queryArgs))
		}

		rows, err := tx.Query(ctx, query, queryArgs...)
		if err != nil {
			return err
		}
		for rows.Next() {
			quotation, scanErr := scanQuotation(rows)
			if scanErr != nil {
				rows.Close()
				return scanErr
			}
			quotations = append(quotations, quotation)
		}
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			return rowsErr
		}

		for i := range quotations {
			items, err := loadQuotationItems(ctx, tx, scope.OrganizationID(), quotations[i].ID)
			if err != nil {
				return err
			}
			quotations[i].Items = items
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return quotations, total, nil
}

func (r *quotationRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateQuotationParams) (domain.Quotation, error) {
	if !scope.IsValid() {
		return domain.Quotation{}, coretenant.ErrInvalidScope
	}

	setClauses := []string{"updated_at = NOW()"}
	var args []interface{}
	addSet := func(column string, value interface{}) {
		args = append(args, value)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if params.DealID != nil {
		addSet("deal_id", nullableString(*params.DealID))
	}
	if params.ContactID != nil {
		addSet("contact_id", nullableString(*params.ContactID))
	}
	if params.CompanyID != nil {
		addSet("company_id", nullableString(*params.CompanyID))
	}
	if params.ValidUntil != nil {
		addSet("valid_until", *params.ValidUntil)
	}
	if params.Notes != nil {
		addSet("notes", *params.Notes)
	}
	if params.UpdatedBy != "" {
		addSet("updated_by", params.UpdatedBy)
	}

	args = append(args, id, scope.OrganizationID())
	query := "UPDATE crm_quotations SET " + strings.Join(setClauses, ", ") +
		fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", len(args)-1, len(args)) +
		quotationColumns

	var quotation domain.Quotation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		status, err := lockQuotationStatus(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		if status != string(domain.QuotationStatusDraft) {
			return ErrQuotationLocked
		}
		var scanErr error
		quotation, scanErr = scanQuotation(tx.QueryRow(ctx, query, args...))
		if scanErr != nil {
			return scanErr
		}
		items, err := loadQuotationItems(ctx, tx, scope.OrganizationID(), quotation.ID)
		if err != nil {
			return err
		}
		quotation.Items = items
		return nil
	})
	if err != nil {
		return domain.Quotation{}, err
	}
	return quotation, nil
}

func (r *quotationRepository) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_quotations
		SET deleted_at = NOW(), updated_by = $1
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		status, err := lockQuotationStatus(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		if status != string(domain.QuotationStatusDraft) {
			return ErrQuotationLocked
		}
		cmdTag, err := tx.Exec(ctx, query, nullableString(deletedBy), id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *quotationRepository) transition(ctx context.Context, scope coretenant.Scope, id string, updatedBy string, fromStatuses []string, toStatus string, timestampColumn string) (domain.Quotation, error) {
	if !scope.IsValid() {
		return domain.Quotation{}, coretenant.ErrInvalidScope
	}

	query := fmt.Sprintf(`
		UPDATE crm_quotations
		SET status = $1, %s = NOW(), updated_by = $2, updated_at = NOW()
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL AND status = ANY($5)
		RETURNING `, timestampColumn) + quotationColumns

	var quotation domain.Quotation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		quotation, scanErr = scanQuotation(tx.QueryRow(ctx, query, toStatus, nullableString(updatedBy), id, scope.OrganizationID(), fromStatuses))
		if scanErr != nil {
			return scanErr
		}
		items, err := loadQuotationItems(ctx, tx, scope.OrganizationID(), quotation.ID)
		if err != nil {
			return err
		}
		quotation.Items = items
		return nil
	})
	if err != nil {
		return domain.Quotation{}, err
	}
	return quotation, nil
}

func (r *quotationRepository) Send(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Quotation, error) {
	return r.transition(ctx, scope, id, updatedBy, []string{string(domain.QuotationStatusDraft)}, string(domain.QuotationStatusSent), "sent_at")
}

func (r *quotationRepository) Approve(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Quotation, error) {
	return r.transition(ctx, scope, id, updatedBy, []string{string(domain.QuotationStatusSent)}, string(domain.QuotationStatusApproved), "approved_at")
}

func (r *quotationRepository) Reject(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Quotation, error) {
	return r.transition(ctx, scope, id, updatedBy, []string{string(domain.QuotationStatusSent), string(domain.QuotationStatusRevisionRequested)}, string(domain.QuotationStatusRejected), "rejected_at")
}

// lockQuotationStatus mengunci baris quotation (FOR UPDATE) dan mengembalikan statusnya.
func lockQuotationStatus(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, id string) (string, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM crm_quotations
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL FOR UPDATE`, id, scope.OrganizationID()).Scan(&status)
	return status, err
}

func (r *quotationRepository) ReplaceItems(ctx context.Context, scope coretenant.Scope, id string, p ReplaceQuotationItemsParams) (domain.Quotation, error) {
	if !scope.IsValid() {
		return domain.Quotation{}, coretenant.ErrInvalidScope
	}
	recurringJSON, err := recurringTotalsJSON(p.RecurringTotals)
	if err != nil {
		return domain.Quotation{}, err
	}
	var q domain.Quotation
	err = r.withTx(ctx, scope, func(tx pgx.Tx) error {
		status, err := lockQuotationStatus(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		if status != string(domain.QuotationStatusDraft) {
			return ErrQuotationLocked
		}
		if _, err := tx.Exec(ctx, `DELETE FROM crm_quotation_items WHERE organization_id = $1 AND quotation_id = $2`, scope.OrganizationID(), id); err != nil {
			return err
		}
		if err := insertQuotationItemsTx(ctx, tx, scope, id, p.Items); err != nil {
			return err
		}
		q, err = scanQuotation(tx.QueryRow(ctx, `UPDATE crm_quotations
			SET subtotal = $1, discount_total = $2, tax_total = $3, grand_total = $4,
				one_time_total = $5, first_invoice_total = $6, recurring_totals = $7, updated_by = $8, updated_at = NOW()
			WHERE id = $9 AND organization_id = $10 RETURNING `+quotationColumns,
			p.Subtotal, p.DiscountTotal, p.TaxTotal, p.GrandTotal,
			totalOrZero(p.OneTimeTotal), totalOrZero(p.FirstInvoiceTotal), recurringJSON,
			nullableString(p.UpdatedBy), id, scope.OrganizationID()))
		if err != nil {
			return err
		}
		q.Items, err = loadQuotationItems(ctx, tx, scope.OrganizationID(), id)
		return err
	})
	return q, err
}

func (r *quotationRepository) Revise(ctx context.Context, scope coretenant.Scope, id string, p CreateQuotationParams) (domain.Quotation, error) {
	if !scope.IsValid() {
		return domain.Quotation{}, coretenant.ErrInvalidScope
	}
	recurringJSON, err := recurringTotalsJSON(p.RecurringTotals)
	if err != nil {
		return domain.Quotation{}, err
	}
	var q domain.Quotation
	err = r.withTx(ctx, scope, func(tx pgx.Tx) error {
		status, err := lockQuotationStatus(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		switch domain.QuotationStatus(status) {
		case domain.QuotationStatusSent, domain.QuotationStatusRejected, domain.QuotationStatusExpired, domain.QuotationStatusRevisionRequested:
		default:
			return ErrQuotationNotRevisable
		}
		if _, err := tx.Exec(ctx, `UPDATE crm_quotations SET status = 'superseded', updated_by = $1, updated_at = NOW()
			WHERE id = $2 AND organization_id = $3`, nullableString(p.CreatedBy), id, scope.OrganizationID()); err != nil {
			return err
		}
		currency := p.Currency
		if currency == "" {
			currency = "IDR"
		}
		q, err = scanQuotation(tx.QueryRow(ctx, `
			INSERT INTO crm_quotations (
				organization_id, deal_id, contact_id, company_id, quotation_number, valid_until,
				subtotal, discount_total, tax_total, grand_total, currency, notes, created_by, revision_of_id, revision_no,
				one_time_total, first_invoice_total, recurring_totals
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
			RETURNING `+quotationColumns,
			scope.OrganizationID(), nullableString(p.DealID), nullableString(p.ContactID), nullableString(p.CompanyID),
			p.QuotationNumber, p.ValidUntil, p.Subtotal, p.DiscountTotal, p.TaxTotal, p.GrandTotal, currency,
			nullableString(p.Notes), nullableString(p.CreatedBy), id, p.RevisionNo,
			totalOrZero(p.OneTimeTotal), totalOrZero(p.FirstInvoiceTotal), recurringJSON))
		if err != nil {
			return err
		}
		if err := insertQuotationItemsTx(ctx, tx, scope, q.ID, p.Items); err != nil {
			return err
		}
		q.Items, err = loadQuotationItems(ctx, tx, scope.OrganizationID(), q.ID)
		return err
	})
	return q, err
}

func (r *quotationRepository) ExpireDue(ctx context.Context, scope coretenant.Scope, today time.Time) (int64, error) {
	if !scope.IsValid() {
		return 0, coretenant.ErrInvalidScope
	}
	var n int64
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE crm_quotations SET status = 'expired', updated_at = NOW()
			WHERE organization_id = $1 AND deleted_at IS NULL AND status = 'sent' AND valid_until < $2::date`,
			scope.OrganizationID(), today.Format("2006-01-02"))
		n = tag.RowsAffected()
		return err
	})
	return n, err
}

func (r *quotationRepository) SetPDFSnapshot(ctx context.Context, scope coretenant.Scope, id, assetID string) (bool, error) {
	if !scope.IsValid() {
		return false, coretenant.ErrInvalidScope
	}
	var ok bool
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE crm_quotations SET pdf_asset_id = $1, pdf_generated_at = NOW()
			WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL AND pdf_asset_id IS NULL`,
			assetID, id, scope.OrganizationID())
		ok = tag.RowsAffected() == 1
		return err
	})
	return ok, err
}

// FindSelfServeByDeal mengembalikan quotation self-serve aktif (draft/approved)
// milik deal beserta item, atau pgx.ErrNoRows.
func (r *quotationRepository) FindSelfServeByDeal(ctx context.Context, scope coretenant.Scope, dealID string) (domain.Quotation, error) {
	if !scope.IsValid() {
		return domain.Quotation{}, coretenant.ErrInvalidScope
	}
	var q domain.Quotation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		q, err = scanQuotation(tx.QueryRow(ctx, `SELECT `+quotationColumns+` FROM crm_quotations
			WHERE organization_id = $1 AND deal_id = $2 AND channel = 'self_serve'
				AND status IN ('draft', 'approved') AND deleted_at IS NULL`, scope.OrganizationID(), dealID))
		if err != nil {
			return err
		}
		q.Items, err = loadQuotationItems(ctx, tx, scope.OrganizationID(), q.ID)
		return err
	})
	return q, err
}

// AcceptSelfServe: draft self-serve → approved (tanpa lewat sent). Selain itu pgx.ErrNoRows.
func (r *quotationRepository) AcceptSelfServe(ctx context.Context, scope coretenant.Scope, id, updatedBy string) (domain.Quotation, error) {
	if !scope.IsValid() {
		return domain.Quotation{}, coretenant.ErrInvalidScope
	}
	var q domain.Quotation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		q, err = scanQuotation(tx.QueryRow(ctx, `UPDATE crm_quotations
			SET status = 'approved', approved_at = NOW(), updated_by = $1, updated_at = NOW()
			WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL AND status = 'draft' AND channel = 'self_serve'
			RETURNING `+quotationColumns, nullableString(updatedBy), id, scope.OrganizationID()))
		if err != nil {
			return err
		}
		q.Items, err = loadQuotationItems(ctx, tx, scope.OrganizationID(), q.ID)
		return err
	})
	return q, err
}
