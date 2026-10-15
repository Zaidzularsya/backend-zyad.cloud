package repository

import (
	"context"
	"errors"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/shared/pricing"
)

var (
	ErrQuotationLocked       = errors.New("quotation is not a draft")
	ErrQuotationNotRevisable = errors.New("only sent, rejected or expired quotations can be revised")
)

// ReplaceQuotationItemsParams mengganti seluruh item draft beserta total header
// yang sudah dihitung service.
type ReplaceQuotationItemsParams struct {
	Items         []QuotationItemInput
	Subtotal      string
	DiscountTotal string
	TaxTotal      string
	GrandTotal    string
	UpdatedBy     string

	OneTimeTotal      string
	FirstInvoiceTotal string
	RecurringTotals   map[pricing.Frequency]string
}

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
	// Snapshot katalog (kosong untuk baris bebas).
	ProductID  string
	SKU        string
	Unit       string
	TaxPercent string
	TaxAmount  string
	Pricing    pricing.Attributes
	Features   []domain.FeatureSnapshot
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
	// Rincian total (lihat domain.Quotation).
	OneTimeTotal      string
	FirstInvoiceTotal string
	RecurringTotals   map[pricing.Frequency]string
	Items             []QuotationItemInput
	CreatedBy         string
	RevisionOfID      string
	RevisionNo        int
	// Channel "" = quotation sales; domain.QuotationChannelSelfServe untuk checkout self-serve.
	Channel string
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
	// ReplaceItems hanya untuk draft (ErrQuotationLocked selain itu).
	ReplaceItems(ctx context.Context, scope coretenant.Scope, id string, p ReplaceQuotationItemsParams) (domain.Quotation, error)
	// Revise menandai id superseded dan membuat draft baru dalam satu transaksi;
	// hanya dari sent/rejected/expired (ErrQuotationNotRevisable). RevisionOfID diisi = id.
	Revise(ctx context.Context, scope coretenant.Scope, id string, p CreateQuotationParams) (domain.Quotation, error)
	// ExpireDue mengubah sent dengan valid_until < today menjadi expired.
	ExpireDue(ctx context.Context, scope coretenant.Scope, today time.Time) (int64, error)
	// SetPDFSnapshot menyimpan snapshot PDF final sekali; false bila sudah ada.
	SetPDFSnapshot(ctx context.Context, scope coretenant.Scope, id, assetID string) (bool, error)
	// FindSelfServeByDeal: quotation self-serve draft/approved milik deal, atau pgx.ErrNoRows.
	FindSelfServeByDeal(ctx context.Context, scope coretenant.Scope, dealID string) (domain.Quotation, error)
	// AcceptSelfServe: draft self-serve → approved; pgx.ErrNoRows bila bukan draft self-serve.
	AcceptSelfServe(ctx context.Context, scope coretenant.Scope, id, updatedBy string) (domain.Quotation, error)
}
