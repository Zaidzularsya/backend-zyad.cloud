package repository

import (
	"context"
	"errors"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

// CreateActivationParams intentionally excludes OrganizationID (taken from
// Scope) and the raw preview token: only its sha256 hash is ever persisted.
type CreateActivationParams struct {
	PageID           string
	InvoiceID        *string
	BuyerName        string
	BuyerContact     string
	BuyerEmail       *string
	Price            string // decimal text, e.g. "150000.00"
	DurationValue    int
	DurationUnit     domain.ActivationDurationUnit
	PreviewTokenHash string
	InvoiceDueAt     time.Time
	CreatedBy        string
}

// ActivationListFilter narrows the reseller's invoice list. Status "" means every status.
type ActivationListFilter struct {
	Status domain.ActivationStatus
	Limit  int
	Offset int
}

// ActivationListItem is an activation with the facts of its page the invoice list shows.
type ActivationListItem struct {
	Activation      domain.LandingPageActivation
	PageTitle       string
	PageSlug        string
	PageStatus      domain.PageStatus
	PageActiveUntil *time.Time
}

// ErrActivationOpen: the page already has an open activation that is not
// overdue (the one-open-per-page partial unique index refused the insert).
var ErrActivationOpen = errors.New("page already has an open activation")

// ActivationRepository stores landing_page_activations. Every query filters
// organization_id from the verified scope (RLS is the second guard).
type ActivationRepository interface {
	ActivationSettler
	Create(ctx context.Context, scope coretenant.Scope, params CreateActivationParams) (domain.LandingPageActivation, error)
	// FindOpenByPage returns pgx.ErrNoRows when the page has no open activation.
	FindOpenByPage(ctx context.Context, scope coretenant.Scope, pageID string) (domain.LandingPageActivation, error)
	// ListOpenByPages returns the open activation of each listed page, keyed by page ID.
	ListOpenByPages(ctx context.Context, scope coretenant.Scope, pageIDs []string) (map[string]domain.LandingPageActivation, error)
	// FindByTokenHash looks up any-status activation by preview token hash
	// within the scope; pgx.ErrNoRows when unknown or owned by another org.
	FindByTokenHash(ctx context.Context, scope coretenant.Scope, tokenHash string) (domain.LandingPageActivation, error)
	// MarkPaid moves an open activation to paid; pgx.ErrNoRows when it is not open.
	MarkPaid(ctx context.Context, scope coretenant.Scope, id string, paidAt time.Time) (domain.LandingPageActivation, error)
	// Void moves an open activation to void; pgx.ErrNoRows when it is not open.
	Void(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingPageActivation, error)
	// CreateReplacingOverdue voids the page's open activation whose invoice is
	// due (invoice_due_at <= now) and inserts the new one in ONE transaction.
	// It returns ErrActivationOpen when an open, not-yet-due activation exists
	// and pgx.ErrNoRows when the page is not in the scope. voided is nil when
	// nothing was replaced.
	CreateReplacingOverdue(ctx context.Context, scope coretenant.Scope, params CreateActivationParams, now time.Time) (created domain.LandingPageActivation, voided *domain.LandingPageActivation, err error)
	// SetInvoice links the receivable invoice to an open activation that has
	// none yet; pgx.ErrNoRows otherwise.
	SetInvoice(ctx context.Context, scope coretenant.Scope, id, invoiceID string) (domain.LandingPageActivation, error)
	// ExistsForInvoice reports whether any activation (any status) of the
	// scope references the invoice.
	ExistsForInvoice(ctx context.Context, scope coretenant.Scope, invoiceID string) (bool, error)
	// FindByID returns any-status activation of the scope; pgx.ErrNoRows when unknown or of another org.
	FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingPageActivation, error)
	// ListByOrganization lists the scope's activations newest first with the page facts, and the total
	// before paging.
	ListByOrganization(ctx context.Context, scope coretenant.Scope, filter ActivationListFilter) ([]ActivationListItem, int64, error)
	// RotatePreview replaces the preview token hash and the invoice due time of an OPEN activation, so
	// the old preview link stops working; pgx.ErrNoRows when it is not open.
	RotatePreview(ctx context.Context, scope coretenant.Scope, id, tokenHash string, invoiceDueAt time.Time) (domain.LandingPageActivation, error)
}

// ActivationSettlement is the outcome of recording a paid activation invoice.
type ActivationSettlement struct {
	// Found: an activation of the scope references the invoice.
	Found bool
	// Applied: this call moved the activation to paid (false for a repeat).
	Applied    bool
	Activation domain.LandingPageActivation
	// PageFound is false when the page is missing or soft-deleted.
	PageFound   bool
	Published   bool // the page was published/extended by this call
	PageSlug    string
	PageTitle   string
	IsHomepage  bool
	ActiveUntil *time.Time
}

// ActivationSettler applies a payment to the activation and its page.
type ActivationSettler interface {
	// SettleByInvoice runs in ONE transaction: lock the activation by
	// (organization_id, invoice_id), and when it is open or void mark it paid
	// and publish the page (page row locked FOR UPDATE). A paid activation or
	// unknown invoice yields Applied=false and changes nothing.
	SettleByInvoice(ctx context.Context, scope coretenant.Scope, invoiceID string, paidAt time.Time) (ActivationSettlement, error)
}
