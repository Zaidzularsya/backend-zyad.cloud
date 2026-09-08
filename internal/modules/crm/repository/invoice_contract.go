package repository

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type InvoiceListFilter struct {
	Status         domain.InvoiceStatus
	DealID         string
	ContactID      string
	CompanyID      string
	QuotationID    string
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// InvoiceItemInput describes one line item at creation time. LineTotal is
// pre-computed by InvoiceService (mirrors QuotationItemInput).
type InvoiceItemInput struct {
	Description     string
	Quantity        string
	UnitPrice       string
	DiscountPercent string
	LineTotal       string
	Position        int
}

// CreateInvoiceParams intentionally excludes OrganizationID. Implementations
// must persist organization identity from Scope. Subtotal/TaxTotal/
// GrandTotal are pre-computed by the service layer from Items.
type CreateInvoiceParams struct {
	QuotationID   string
	DealID        string
	ContactID     string
	CompanyID     string
	InvoiceNumber string
	IssueDate     *time.Time
	DueDate       *time.Time
	Subtotal      string
	TaxTotal      string
	GrandTotal    string
	Currency      string
	Items         []InvoiceItemInput
	CreatedBy     string
}

type UpdateInvoiceParams struct {
	QuotationID *string
	DealID      *string
	ContactID   *string
	CompanyID   *string
	IssueDate   *time.Time
	DueDate     *time.Time
	UpdatedBy   string
}

// InvoiceRepository is the tenant-owned data contract. Every method requires
// a verified immutable scope and row lookups include both scope and resource ID.
type InvoiceRepository interface {
	Create(context.Context, coretenant.Scope, CreateInvoiceParams) (domain.Invoice, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.Invoice, error)
	List(context.Context, coretenant.Scope, InvoiceListFilter) ([]domain.Invoice, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateInvoiceParams) (domain.Invoice, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Send(context.Context, coretenant.Scope, string, string) (domain.Invoice, error)
	MarkPaid(context.Context, coretenant.Scope, string, string, string) (domain.Invoice, error)
	Cancel(context.Context, coretenant.Scope, string, string) (domain.Invoice, error)
}
