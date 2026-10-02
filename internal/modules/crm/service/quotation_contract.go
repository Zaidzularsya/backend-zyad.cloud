package service

import (
	"context"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

// QuotationLineInput is the raw line item a caller submits. priceQuotationLines
// computes line/header totals from it with per-line half-up rounding.
type QuotationLineInput struct {
	ProductID       string // S3: snapshot katalog bila diisi
	Description     string
	Quantity        string
	UnitPrice       string
	DiscountPercent string
	TaxPercent      string
	Unit            string
	SKU             string // diisi service dari katalog; klien tidak mengirim
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
	// Items nil = item tidak diubah; non-nil mengganti seluruh item (hanya draft).
	Items []QuotationLineInput
}

// CatalogProducts adalah subset katalog yang dipakai quotation untuk snapshot item.
type CatalogProducts interface {
	FindByIDs(ctx context.Context, scope coretenant.Scope, ids []string) (map[string]catalogdomain.Product, error)
}

type QuotationServiceOption func(*quotationService)

type QuotationService interface {
	Create(context.Context, coretenant.Scope, CreateQuotationInput) (domain.Quotation, error)
	Get(context.Context, coretenant.Scope, string) (domain.Quotation, error)
	List(context.Context, coretenant.Scope, repository.QuotationListFilter) ([]domain.Quotation, int64, error)
	Update(context.Context, coretenant.Scope, string, UpdateQuotationInput) (domain.Quotation, error)
	Delete(context.Context, coretenant.Scope, string, string) error
	Send(context.Context, coretenant.Scope, string, string) (domain.Quotation, error)
	Approve(context.Context, coretenant.Scope, string, string) (domain.Quotation, error)
	Reject(context.Context, coretenant.Scope, string, string) (domain.Quotation, error)
	// Revise membuat draft {nomor akar}-R{n} dari quotation sent/rejected/expired.
	Revise(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.Quotation, error)
}
