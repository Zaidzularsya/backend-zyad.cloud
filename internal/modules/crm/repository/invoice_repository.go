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

type invoiceRepository struct {
	db *database.Pool
}

func NewInvoiceRepository(db *database.Pool) InvoiceRepository {
	return &invoiceRepository{db: db}
}

func (r *invoiceRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

const invoiceColumns = `
	id, organization_id, quotation_id, deal_id, contact_id, company_id, invoice_number, status,
	issue_date, due_date, subtotal::text, tax_total::text, grand_total::text, amount_paid::text,
	paid_at, currency, created_by, updated_by, created_at, updated_at, deleted_at
`

const invoiceItemColumns = `
	id, description, quantity::text, unit_price::text, discount_percent::text, line_total::text, position
`

func scanInvoice(row pgx.Row) (domain.Invoice, error) {
	var inv domain.Invoice
	var quotationID, dealID, contactID, companyID *string
	var createdBy, updatedBy *string
	var status string

	err := row.Scan(
		&inv.ID, &inv.OrganizationID, &quotationID, &dealID, &contactID, &companyID, &inv.InvoiceNumber, &status,
		&inv.IssueDate, &inv.DueDate, &inv.Subtotal, &inv.TaxTotal, &inv.GrandTotal, &inv.AmountPaid,
		&inv.PaidAt, &inv.Currency, &createdBy, &updatedBy, &inv.CreatedAt, &inv.UpdatedAt, &inv.DeletedAt,
	)
	if err != nil {
		return domain.Invoice{}, err
	}

	inv.Status = domain.InvoiceStatus(status)
	inv.QuotationID = quotationID
	inv.DealID = dealID
	inv.ContactID = contactID
	inv.CompanyID = companyID
	if createdBy != nil {
		inv.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		inv.UpdatedBy = *updatedBy
	}

	return inv, nil
}

func scanInvoiceItem(row pgx.Row) (domain.InvoiceItem, error) {
	var item domain.InvoiceItem
	err := row.Scan(&item.ID, &item.Description, &item.Quantity, &item.UnitPrice, &item.DiscountPercent, &item.LineTotal, &item.Position)
	return item, err
}

func loadInvoiceItems(ctx context.Context, tx pgx.Tx, organizationID string, invoiceID string) ([]domain.InvoiceItem, error) {
	query := `SELECT ` + invoiceItemColumns + ` FROM crm_invoice_items
		WHERE organization_id = $1 AND invoice_id = $2
		ORDER BY position ASC`

	rows, err := tx.Query(ctx, query, organizationID, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.InvoiceItem, 0)
	for rows.Next() {
		item, err := scanInvoiceItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *invoiceRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateInvoiceParams) (domain.Invoice, error) {
	if !scope.IsValid() {
		return domain.Invoice{}, coretenant.ErrInvalidScope
	}

	currency := params.Currency
	if currency == "" {
		currency = "IDR"
	}

	query := `
		INSERT INTO crm_invoices (
			organization_id, quotation_id, deal_id, contact_id, company_id, invoice_number,
			issue_date, due_date, subtotal, tax_total, grand_total, currency, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		) RETURNING ` + invoiceColumns

	var invoice domain.Invoice
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		invoice, scanErr = scanInvoice(tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			nullableString(params.QuotationID),
			nullableString(params.DealID),
			nullableString(params.ContactID),
			nullableString(params.CompanyID),
			params.InvoiceNumber,
			params.IssueDate,
			params.DueDate,
			params.Subtotal,
			params.TaxTotal,
			params.GrandTotal,
			currency,
			nullableString(params.CreatedBy),
		))
		if scanErr != nil {
			return scanErr
		}

		for _, item := range params.Items {
			_, err := tx.Exec(ctx, `
				INSERT INTO crm_invoice_items (
					organization_id, invoice_id, description, quantity, unit_price, discount_percent, line_total, position
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`, scope.OrganizationID(), invoice.ID, item.Description, item.Quantity, item.UnitPrice, nullableString(item.DiscountPercent), item.LineTotal, item.Position)
			if err != nil {
				return err
			}
		}

		items, err := loadInvoiceItems(ctx, tx, scope.OrganizationID(), invoice.ID)
		if err != nil {
			return err
		}
		invoice.Items = items
		return nil
	})
	if err != nil {
		return domain.Invoice{}, err
	}
	return invoice, nil
}

func (r *invoiceRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Invoice, error) {
	if !scope.IsValid() {
		return domain.Invoice{}, coretenant.ErrInvalidScope
	}

	query := `SELECT ` + invoiceColumns + ` FROM crm_invoices WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	var invoice domain.Invoice
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		invoice, scanErr = scanInvoice(tx.QueryRow(ctx, query, id, scope.OrganizationID()))
		if scanErr != nil {
			return scanErr
		}
		items, err := loadInvoiceItems(ctx, tx, scope.OrganizationID(), invoice.ID)
		if err != nil {
			return err
		}
		invoice.Items = items
		return nil
	})
	if err != nil {
		return domain.Invoice{}, err
	}
	return invoice, nil
}

func (r *invoiceRepository) List(ctx context.Context, scope coretenant.Scope, filter InvoiceListFilter) ([]domain.Invoice, int64, error) {
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
	if filter.QuotationID != "" {
		args = append(args, filter.QuotationID)
		whereClauses = append(whereClauses, fmt.Sprintf("quotation_id = $%d", len(args)))
	}

	where := strings.Join(whereClauses, " AND ")
	countQuery := "SELECT COUNT(*) FROM crm_invoices WHERE " + where

	var invoices []domain.Invoice
	var total int64

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			invoices = []domain.Invoice{}
			return nil
		}

		query := "SELECT " + invoiceColumns + " FROM crm_invoices WHERE " + where + " ORDER BY created_at DESC"
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
			invoice, scanErr := scanInvoice(rows)
			if scanErr != nil {
				rows.Close()
				return scanErr
			}
			invoices = append(invoices, invoice)
		}
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			return rowsErr
		}

		for i := range invoices {
			items, err := loadInvoiceItems(ctx, tx, scope.OrganizationID(), invoices[i].ID)
			if err != nil {
				return err
			}
			invoices[i].Items = items
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return invoices, total, nil
}

func (r *invoiceRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateInvoiceParams) (domain.Invoice, error) {
	if !scope.IsValid() {
		return domain.Invoice{}, coretenant.ErrInvalidScope
	}

	setClauses := []string{"updated_at = NOW()"}
	var args []interface{}
	addSet := func(column string, value interface{}) {
		args = append(args, value)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if params.QuotationID != nil {
		addSet("quotation_id", nullableString(*params.QuotationID))
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
	if params.IssueDate != nil {
		addSet("issue_date", *params.IssueDate)
	}
	if params.DueDate != nil {
		addSet("due_date", *params.DueDate)
	}
	if params.UpdatedBy != "" {
		addSet("updated_by", params.UpdatedBy)
	}

	args = append(args, id, scope.OrganizationID())
	query := "UPDATE crm_invoices SET " + strings.Join(setClauses, ", ") +
		fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", len(args)-1, len(args)) +
		invoiceColumns

	var invoice domain.Invoice
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		invoice, scanErr = scanInvoice(tx.QueryRow(ctx, query, args...))
		if scanErr != nil {
			return scanErr
		}
		items, err := loadInvoiceItems(ctx, tx, scope.OrganizationID(), invoice.ID)
		if err != nil {
			return err
		}
		invoice.Items = items
		return nil
	})
	if err != nil {
		return domain.Invoice{}, err
	}
	return invoice, nil
}

func (r *invoiceRepository) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_invoices
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

func (r *invoiceRepository) Send(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Invoice, error) {
	if !scope.IsValid() {
		return domain.Invoice{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_invoices
		SET status = 'sent', updated_by = $1, updated_at = NOW()
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL AND status = 'draft'
		RETURNING ` + invoiceColumns

	var invoice domain.Invoice
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		invoice, scanErr = scanInvoice(tx.QueryRow(ctx, query, nullableString(updatedBy), id, scope.OrganizationID()))
		if scanErr != nil {
			return scanErr
		}
		items, err := loadInvoiceItems(ctx, tx, scope.OrganizationID(), invoice.ID)
		if err != nil {
			return err
		}
		invoice.Items = items
		return nil
	})
	if err != nil {
		return domain.Invoice{}, err
	}
	return invoice, nil
}

func (r *invoiceRepository) MarkPaid(ctx context.Context, scope coretenant.Scope, id string, amountPaid string, updatedBy string) (domain.Invoice, error) {
	if !scope.IsValid() {
		return domain.Invoice{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_invoices
		SET status = 'paid', amount_paid = $1, paid_at = NOW(), updated_by = $2, updated_at = NOW()
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL AND status IN ('sent', 'overdue')
		RETURNING ` + invoiceColumns

	var invoice domain.Invoice
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		invoice, scanErr = scanInvoice(tx.QueryRow(ctx, query, amountPaid, nullableString(updatedBy), id, scope.OrganizationID()))
		if scanErr != nil {
			return scanErr
		}
		items, err := loadInvoiceItems(ctx, tx, scope.OrganizationID(), invoice.ID)
		if err != nil {
			return err
		}
		invoice.Items = items
		return nil
	})
	if err != nil {
		return domain.Invoice{}, err
	}
	return invoice, nil
}

func (r *invoiceRepository) Cancel(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Invoice, error) {
	if !scope.IsValid() {
		return domain.Invoice{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_invoices
		SET status = 'cancelled', updated_by = $1, updated_at = NOW()
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL AND status <> 'paid'
		RETURNING ` + invoiceColumns

	var invoice domain.Invoice
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		invoice, scanErr = scanInvoice(tx.QueryRow(ctx, query, nullableString(updatedBy), id, scope.OrganizationID()))
		if scanErr != nil {
			return scanErr
		}
		items, err := loadInvoiceItems(ctx, tx, scope.OrganizationID(), invoice.ID)
		if err != nil {
			return err
		}
		invoice.Items = items
		return nil
	})
	if err != nil {
		return domain.Invoice{}, err
	}
	return invoice, nil
}
