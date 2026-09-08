package service

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

// QuotationLineInput is the raw (unpriced-total) line item a caller submits.
// QuotationService.computeTotals fills in LineTotal/Subtotal/DiscountTotal/
// GrandTotal from these before handing off to the repository — see that
// method's doc comment for the float64-arithmetic simplification this
// implies.
type QuotationLineInput struct {
	Description     string
	Quantity        string
	UnitPrice       string
	DiscountPercent string
}

type CreateQuotationInput struct {
	DealID          string
	ContactID       string
	CompanyID       string
	QuotationNumber string
	ValidUntil      *time.Time
	Currency        string
	Notes           string
	TaxTotal        string
	Items           []QuotationLineInput
	CreatedBy       string
}

type UpdateQuotationInput struct {
	DealID     *string
	ContactID  *string
	CompanyID  *string
	ValidUntil *time.Time
	Notes      *string
	UpdatedBy  string
}

type QuotationService interface {
	Create(context.Context, coretenant.Scope, CreateQuotationInput) (domain.Quotation, error)
	Get(context.Context, coretenant.Scope, string) (domain.Quotation, error)
	List(context.Context, coretenant.Scope, repository.QuotationListFilter) ([]domain.Quotation, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateQuotationInput) (domain.Quotation, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Send(context.Context, coretenant.Scope, string, string) (domain.Quotation, error)
	Approve(context.Context, coretenant.Scope, string, string) (domain.Quotation, error)
	Reject(context.Context, coretenant.Scope, string, string) (domain.Quotation, error)
}
