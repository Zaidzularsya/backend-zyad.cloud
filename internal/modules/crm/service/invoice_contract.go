package service

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

// InvoiceLineInput mirrors QuotationLineInput.
type InvoiceLineInput struct {
	Description     string
	Quantity        string
	UnitPrice       string
	DiscountPercent string
}

// CreateInvoiceInput either takes Items directly, or (when QuotationID is set
// and Items is empty) copies items/totals from an existing approved
// quotation — see InvoiceService.Create for the copy behavior.
type CreateInvoiceInput struct {
	QuotationID   string
	DealID        string
	ContactID     string
	CompanyID     string
	InvoiceNumber string
	IssueDate     *time.Time
	DueDate       *time.Time
	Currency      string
	TaxTotal      string
	Items         []InvoiceLineInput
	CreatedBy     string
}

type UpdateInvoiceInput struct {
	QuotationID *string
	DealID      *string
	ContactID   *string
	CompanyID   *string
	IssueDate   *time.Time
	DueDate     *time.Time
	UpdatedBy   string
}

type InvoiceService interface {
	Create(context.Context, coretenant.Scope, CreateInvoiceInput) (domain.Invoice, error)
	Get(context.Context, coretenant.Scope, string) (domain.Invoice, error)
	List(context.Context, coretenant.Scope, repository.InvoiceListFilter) ([]domain.Invoice, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateInvoiceInput) (domain.Invoice, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Send(context.Context, coretenant.Scope, string, string) (domain.Invoice, error)
	MarkPaid(context.Context, coretenant.Scope, string, string, string) (domain.Invoice, error)
	Cancel(context.Context, coretenant.Scope, string, string) (domain.Invoice, error)
}
