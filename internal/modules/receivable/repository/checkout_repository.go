package repository

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
)

// Checkout adalah satu sesi DOKU Checkout untuk invoice. Nomor DOKU sama untuk
// semua sesi satu invoice, jadi FindByDokuNumber mengambil sesi terbaru.
type Checkout struct {
	ID, OrganizationID, InvoiceID, DokuInvoiceNumber, PaymentURL, Status string // pending|paid|expired|failed
	Amount                                                               string
	ExpiresAt                                                            time.Time
	CreatedAt                                                            time.Time
}

// CheckoutRepository memakai tabel directory tanpa RLS (webhook mencari organisasi dari nomor DOKU).
type CheckoutRepository interface {
	// FindReusable: sesi pending terbaru yang belum kedaluwarsa; pgx.ErrNoRows bila tidak ada.
	FindReusable(ctx context.Context, scope coretenant.Scope, invoiceID string, now time.Time) (Checkout, error)
	Create(ctx context.Context, scope coretenant.Scope, c Checkout) (Checkout, error)
	// FindByDokuNumber tanpa scope (directory); pgx.ErrNoRows bila tidak ada.
	FindByDokuNumber(ctx context.Context, dokuNumber string) (Checkout, error)
	SetStatus(ctx context.Context, id, status string) error
}

type checkoutRepository struct{ db *database.Pool }

func NewCheckoutRepository(db *database.Pool) CheckoutRepository { return &checkoutRepository{db: db} }

const checkoutColumns = `id::text, organization_id::text, invoice_id::text, doku_invoice_number, amount::text, payment_url, status, expires_at, created_at`

func (r *checkoutRepository) FindReusable(ctx context.Context, scope coretenant.Scope, invoiceID string, now time.Time) (Checkout, error) {
	if !scope.IsValid() {
		return Checkout{}, coretenant.ErrInvalidScope
	}
	var c Checkout
	err := r.db.QueryRow(ctx, `
		SELECT `+checkoutColumns+` FROM receivable_checkouts
		WHERE organization_id = $1 AND invoice_id = $2 AND status = 'pending' AND expires_at > $3
		ORDER BY created_at DESC LIMIT 1`, scope.OrganizationID(), invoiceID, now).
		Scan(&c.ID, &c.OrganizationID, &c.InvoiceID, &c.DokuInvoiceNumber, &c.Amount, &c.PaymentURL, &c.Status, &c.ExpiresAt, &c.CreatedAt)
	return c, err
}

func (r *checkoutRepository) Create(ctx context.Context, scope coretenant.Scope, c Checkout) (Checkout, error) {
	if !scope.IsValid() {
		return Checkout{}, coretenant.ErrInvalidScope
	}
	var out Checkout
	err := r.db.QueryRow(ctx, `
		INSERT INTO receivable_checkouts (organization_id, invoice_id, doku_invoice_number, amount, payment_url, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+checkoutColumns,
		scope.OrganizationID(), c.InvoiceID, c.DokuInvoiceNumber, c.Amount, c.PaymentURL, c.ExpiresAt).
		Scan(&out.ID, &out.OrganizationID, &out.InvoiceID, &out.DokuInvoiceNumber, &out.Amount, &out.PaymentURL, &out.Status, &out.ExpiresAt, &out.CreatedAt)
	return out, err
}

func (r *checkoutRepository) FindByDokuNumber(ctx context.Context, dokuNumber string) (Checkout, error) {
	var c Checkout
	err := r.db.QueryRow(ctx, `
		SELECT `+checkoutColumns+` FROM receivable_checkouts
		WHERE doku_invoice_number = $1 ORDER BY created_at DESC LIMIT 1`, dokuNumber).
		Scan(&c.ID, &c.OrganizationID, &c.InvoiceID, &c.DokuInvoiceNumber, &c.Amount, &c.PaymentURL, &c.Status, &c.ExpiresAt, &c.CreatedAt)
	return c, err
}

func (r *checkoutRepository) SetStatus(ctx context.Context, id, status string) error {
	_, err := r.db.Exec(ctx, `UPDATE receivable_checkouts SET status = $2 WHERE id = $1`, id, status)
	return err
}
