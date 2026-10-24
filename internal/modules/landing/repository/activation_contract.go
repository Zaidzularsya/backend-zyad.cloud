package repository

import (
	"context"
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

// ActivationRepository stores landing_page_activations. Every query filters
// organization_id from the verified scope (RLS is the second guard).
type ActivationRepository interface {
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
}
