package repository

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type QuotationListFilter struct {
	Status         domain.QuotationStatus
	DealID         string
	ContactID      string
	CompanyID      string
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// QuotationItemInput describes one line item at creation time. LineTotal is
// pre-computed by QuotationService (see domain.Quotation doc comment).
type QuotationItemInput struct {
	Description     string
	Quantity        string
	UnitPrice       string
	DiscountPercent string
	LineTotal       string
	Position        int
}

// CreateQuotationParams intentionally excludes OrganizationID. Implementations
// must persist organization identity from Scope. Subtotal/DiscountTotal/
// TaxTotal/GrandTotal are pre-computed by the service layer from Items.
type CreateQuotationParams struct {
	DealID          string
	ContactID       string
	CompanyID       string
	QuotationNumber string
	ValidUntil      *time.Time
	Subtotal        string
	DiscountTotal   string
	TaxTotal        string
	GrandTotal      string
	Currency        string
	Notes           string
	Items           []QuotationItemInput
	CreatedBy       string
}

type UpdateQuotationParams struct {
	DealID     *string
	ContactID  *string
	CompanyID  *string
	ValidUntil *time.Time
	Notes      *string
	UpdatedBy  string
}

// QuotationRepository is the tenant-owned data contract. Every method
// requires a verified immutable scope and row lookups include both scope and
// resource ID.
type QuotationRepository interface {
	Create(context.Context, coretenant.Scope, CreateQuotationParams) (domain.Quotation, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.Quotation, error)
	List(context.Context, coretenant.Scope, QuotationListFilter) ([]domain.Quotation, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateQuotationParams) (domain.Quotation, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Send(context.Context, coretenant.Scope, string, string) (domain.Quotation, error)
	Approve(context.Context, coretenant.Scope, string, string) (domain.Quotation, error)
	Reject(context.Context, coretenant.Scope, string, string) (domain.Quotation, error)
}
