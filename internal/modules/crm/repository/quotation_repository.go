package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
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
	sent_at, approved_at, rejected_at, created_by, updated_by, created_at, updated_at, deleted_at
`

const quotationItemColumns = `
	id, description, quantity::text, unit_price::text, discount_percent::text, line_total::text, position
`

func scanQuotation(row pgx.Row) (domain.Quotation, error) {
	var q domain.Quotation
	var dealID, contactID, companyID *string
	var notes *string
	var createdBy, updatedBy *string
	var status string

	err := row.Scan(
		&q.ID, &q.OrganizationID, &dealID, &contactID, &companyID, &q.QuotationNumber, &status, &q.ValidUntil,
		&q.Subtotal, &q.DiscountTotal, &q.TaxTotal, &q.GrandTotal, &q.Currency, &notes,
		&q.SentAt, &q.ApprovedAt, &q.RejectedAt, &createdBy, &updatedBy, &q.CreatedAt, &q.UpdatedAt, &q.DeletedAt,
	)
	if err != nil {
		return domain.Quotation{}, err
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
	err := row.Scan(&item.ID, &item.Description, &item.Quantity, &item.UnitPrice, &item.DiscountPercent, &item.LineTotal, &item.Position)
	return item, err
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
			subtotal, discount_total, tax_total, grand_total, currency, notes, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		) RETURNING ` + quotationColumns

	var quotation domain.Quotation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
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
		))
		if scanErr != nil {
			return scanErr
		}

		for _, item := range params.Items {
			_, err := tx.Exec(ctx, `
				INSERT INTO crm_quotation_items (
					organization_id, quotation_id, description, quantity, unit_price, discount_percent, line_total, position
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`, scope.OrganizationID(), quotation.ID, item.Description, item.Quantity, item.UnitPrice, nullableString(item.DiscountPercent), item.LineTotal, item.Position)
			if err != nil {
				return err
			}
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

func (r *quotationRepository) transition(ctx context.Context, scope coretenant.Scope, id string, updatedBy string, fromStatus string, toStatus string, timestampColumn string) (domain.Quotation, error) {
	if !scope.IsValid() {
		return domain.Quotation{}, coretenant.ErrInvalidScope
	}

	query := fmt.Sprintf(`
		UPDATE crm_quotations
		SET status = $1, %s = NOW(), updated_by = $2, updated_at = NOW()
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL AND status = $5
		RETURNING `, timestampColumn) + quotationColumns

	var quotation domain.Quotation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		quotation, scanErr = scanQuotation(tx.QueryRow(ctx, query, toStatus, nullableString(updatedBy), id, scope.OrganizationID(), fromStatus))
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
	return r.transition(ctx, scope, id, updatedBy, string(domain.QuotationStatusDraft), string(domain.QuotationStatusSent), "sent_at")
}

func (r *quotationRepository) Approve(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Quotation, error) {
	return r.transition(ctx, scope, id, updatedBy, string(domain.QuotationStatusSent), string(domain.QuotationStatusApproved), "approved_at")
}

func (r *quotationRepository) Reject(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Quotation, error) {
	return r.transition(ctx, scope, id, updatedBy, string(domain.QuotationStatusSent), string(domain.QuotationStatusRejected), "rejected_at")
}
