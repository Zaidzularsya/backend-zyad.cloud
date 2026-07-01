package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/platform/database"
)

const invoiceSelectColumns = `
	id,
	organization_id,
	subscription_id,
	invoice_number,
	status,
	currency,
	subtotal_amount::text,
	discount_amount::text,
	tax_amount::text,
	total_amount::text,
	due_date,
	paid_at,
	metadata,
	created_at,
	updated_at
`

const invoiceItemSelectColumns = `
	id,
	invoice_id,
	item_type,
	description,
	quantity::text,
	unit_amount::text,
	total_amount::text,
	metadata,
	created_at,
	updated_at
`

type InvoiceRepository struct {
	db *database.Pool
}

type InvoiceListFilter struct {
	OrganizationID string
	SubscriptionID string
	Status         model.InvoiceStatus
	Limit          int
	Offset         int
}

type CreateInvoiceParams struct {
	OrganizationID string
	SubscriptionID *string
	InvoiceNumber  string
	Status         model.InvoiceStatus
	Currency       string
	SubtotalAmount string
	DiscountAmount string
	TaxAmount      string
	TotalAmount    string
	DueDate        *time.Time
	PaidAt         *time.Time
	Metadata       map[string]any
	Items          []CreateInvoiceItemParams
}

type CreateInvoiceItemParams struct {
	Type        model.InvoiceItemType
	Description string
	Quantity    string
	UnitAmount  string
	TotalAmount string
	Metadata    map[string]any
}

type UpdateInvoiceStatusParams struct {
	ID             string
	OrganizationID string
	Status         model.InvoiceStatus
	PaidAt         *time.Time
}

func NewInvoiceRepository(db *database.Pool) *InvoiceRepository {
	return &InvoiceRepository{db: db}
}

func (r *InvoiceRepository) Create(ctx context.Context, params CreateInvoiceParams) (model.Invoice, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.Invoice{}, err
	}
	defer tx.Rollback(ctx)

	metadata, err := encodeMap(params.Metadata)
	if err != nil {
		return model.Invoice{}, err
	}
	status := params.Status
	if status == "" {
		status = model.InvoiceStatusDraft
	}
	var invoice model.Invoice
	var metadataBytes []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO billing_invoices (
			organization_id,
			subscription_id,
			invoice_number,
			status,
			currency,
			subtotal_amount,
			discount_amount,
			tax_amount,
			total_amount,
			due_date,
			paid_at,
			metadata
		)
		VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, $4, $5, $6::numeric, $7::numeric, $8::numeric, $9::numeric, $10, $11, $12::jsonb)
		RETURNING `+invoiceSelectColumns,
		strings.TrimSpace(params.OrganizationID),
		stringPointerValue(params.SubscriptionID),
		strings.TrimSpace(params.InvoiceNumber),
		string(status),
		upperOrDefault(params.Currency, "IDR"),
		amountOrDefault(params.SubtotalAmount),
		amountOrDefault(params.DiscountAmount),
		amountOrDefault(params.TaxAmount),
		amountOrDefault(params.TotalAmount),
		params.DueDate,
		params.PaidAt,
		metadata,
	).Scan(invoiceScanDest(&invoice, &metadataBytes)...)
	if err != nil {
		return model.Invoice{}, err
	}
	if err := decodeMap(metadataBytes, &invoice.Metadata); err != nil {
		return model.Invoice{}, err
	}

	for _, item := range params.Items {
		if _, err := insertInvoiceItem(ctx, tx, invoice.ID, item); err != nil {
			return model.Invoice{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Invoice{}, err
	}
	return invoice, nil
}

func (r *InvoiceRepository) FindByID(ctx context.Context, organizationID string, id string) (model.Invoice, error) {
	var invoice model.Invoice
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		SELECT `+invoiceSelectColumns+`
		FROM billing_invoices
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
	`, strings.TrimSpace(id), strings.TrimSpace(organizationID)).Scan(invoiceScanDest(&invoice, &metadataBytes)...)
	if err != nil {
		return model.Invoice{}, err
	}
	if err := decodeMap(metadataBytes, &invoice.Metadata); err != nil {
		return model.Invoice{}, err
	}
	return invoice, nil
}

func (r *InvoiceRepository) FindByIDUnscoped(ctx context.Context, id string) (model.Invoice, error) {
	var invoice model.Invoice
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		SELECT `+invoiceSelectColumns+`
		FROM billing_invoices
		WHERE id = $1::uuid
	`, strings.TrimSpace(id)).Scan(invoiceScanDest(&invoice, &metadataBytes)...)
	if err != nil {
		return model.Invoice{}, err
	}
	if err := decodeMap(metadataBytes, &invoice.Metadata); err != nil {
		return model.Invoice{}, err
	}
	return invoice, nil
}

func (r *InvoiceRepository) List(ctx context.Context, filter InvoiceListFilter) ([]model.Invoice, int64, error) {
	where, args := invoiceWhere(filter)
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM billing_invoices"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := pagination(filter.Limit, filter.Offset)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+invoiceSelectColumns+`
		FROM billing_invoices`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	invoices := make([]model.Invoice, 0)
	for rows.Next() {
		var invoice model.Invoice
		var metadataBytes []byte
		if err := rows.Scan(invoiceScanDest(&invoice, &metadataBytes)...); err != nil {
			return nil, 0, err
		}
		if err := decodeMap(metadataBytes, &invoice.Metadata); err != nil {
			return nil, 0, err
		}
		invoices = append(invoices, invoice)
	}
	return invoices, total, rows.Err()
}

func (r *InvoiceRepository) ListItems(ctx context.Context, invoiceID string) ([]model.InvoiceItem, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+invoiceItemSelectColumns+`
		FROM billing_invoice_items
		WHERE invoice_id = $1::uuid
		ORDER BY created_at ASC, id ASC
	`, strings.TrimSpace(invoiceID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.InvoiceItem, 0)
	for rows.Next() {
		item, err := scanInvoiceItem(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *InvoiceRepository) UpdateStatus(ctx context.Context, params UpdateInvoiceStatusParams) (model.Invoice, error) {
	var invoice model.Invoice
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		UPDATE billing_invoices
		SET
			status = $3,
			paid_at = CASE WHEN $4::boolean THEN $5 ELSE paid_at END,
			updated_at = now()
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
		RETURNING `+invoiceSelectColumns,
		strings.TrimSpace(params.ID),
		strings.TrimSpace(params.OrganizationID),
		string(params.Status),
		params.PaidAt != nil,
		params.PaidAt,
	).Scan(invoiceScanDest(&invoice, &metadataBytes)...)
	if err != nil {
		return model.Invoice{}, err
	}
	if err := decodeMap(metadataBytes, &invoice.Metadata); err != nil {
		return model.Invoice{}, err
	}
	return invoice, nil
}

func invoiceWhere(filter InvoiceListFilter) (string, []any) {
	conditions := []string{}
	args := []any{}
	if filter.OrganizationID != "" {
		args = append(args, strings.TrimSpace(filter.OrganizationID))
		conditions = append(conditions, fmt.Sprintf("organization_id = $%d::uuid", len(args)))
	}
	if filter.SubscriptionID != "" {
		args = append(args, strings.TrimSpace(filter.SubscriptionID))
		conditions = append(conditions, fmt.Sprintf("subscription_id = $%d::uuid", len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func invoiceScanDest(invoice *model.Invoice, metadata *[]byte) []any {
	return []any{
		&invoice.ID,
		&invoice.OrganizationID,
		&invoice.SubscriptionID,
		&invoice.InvoiceNumber,
		&invoice.Status,
		&invoice.Currency,
		&invoice.SubtotalAmount,
		&invoice.DiscountAmount,
		&invoice.TaxAmount,
		&invoice.TotalAmount,
		&invoice.DueDate,
		&invoice.PaidAt,
		metadata,
		&invoice.CreatedAt,
		&invoice.UpdatedAt,
	}
}

func amountOrDefault(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0"
	}
	return value
}

func insertInvoiceItem(
	ctx context.Context,
	tx pgx.Tx,
	invoiceID string,
	params CreateInvoiceItemParams,
) (model.InvoiceItem, error) {
	metadata, err := encodeMap(params.Metadata)
	if err != nil {
		return model.InvoiceItem{}, err
	}
	var item model.InvoiceItem
	var metadataBytes []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO billing_invoice_items (
			invoice_id,
			item_type,
			description,
			quantity,
			unit_amount,
			total_amount,
			metadata
		)
		VALUES ($1::uuid, $2, $3, $4::numeric, $5::numeric, $6::numeric, $7::jsonb)
		RETURNING `+invoiceItemSelectColumns,
		strings.TrimSpace(invoiceID),
		string(params.Type),
		strings.TrimSpace(params.Description),
		amountOrDefault(params.Quantity),
		amountOrDefault(params.UnitAmount),
		amountOrDefault(params.TotalAmount),
		metadata,
	).Scan(invoiceItemScanDest(&item, &metadataBytes)...)
	if err != nil {
		return model.InvoiceItem{}, err
	}
	if err := decodeMap(metadataBytes, &item.Metadata); err != nil {
		return model.InvoiceItem{}, err
	}
	return item, nil
}

func scanInvoiceItem(scan func(dest ...any) error) (model.InvoiceItem, error) {
	var item model.InvoiceItem
	var metadataBytes []byte
	if err := scan(invoiceItemScanDest(&item, &metadataBytes)...); err != nil {
		return model.InvoiceItem{}, err
	}
	if err := decodeMap(metadataBytes, &item.Metadata); err != nil {
		return model.InvoiceItem{}, err
	}
	return item, nil
}

func invoiceItemScanDest(item *model.InvoiceItem, metadata *[]byte) []any {
	return []any{
		&item.ID,
		&item.InvoiceID,
		&item.Type,
		&item.Description,
		&item.Quantity,
		&item.UnitAmount,
		&item.TotalAmount,
		metadata,
		&item.CreatedAt,
		&item.UpdatedAt,
	}
}
