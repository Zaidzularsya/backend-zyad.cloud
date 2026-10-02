package service

import (
	"context"
	"io"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/quotationpdf"
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

// QuotationFileStore: subset asset service untuk snapshot PDF privat.
type QuotationFileStore interface {
	UploadObject(ctx context.Context, scope coretenant.Scope, params assetservice.UploadObjectParams, content io.Reader) (assetdomain.AssetObject, error)
	OpenObject(ctx context.Context, scope coretenant.Scope, id string) (io.ReadCloser, assetdomain.AssetObject, error)
	DeleteObject(ctx context.Context, scope coretenant.Scope, id string) error
}

type QuotationPDFRenderer interface {
	Render(quotationpdf.Document) ([]byte, error)
}

type QuotationDocumentDeps struct {
	Issuers    repository.QuotationIssuerRepository
	Contacts   repository.ContactRepository
	Companies  repository.CompanyRepository
	Activities repository.ActivityRepository
	Files      QuotationFileStore
	Renderer   QuotationPDFRenderer
}

type QuotationChannelDeps struct {
	Sends    repository.QuotationSendRepository
	Email    QuotationEmailSender    // nil = kanal tidak tersedia
	WhatsApp QuotationWhatsAppSender // nil = kanal tidak tersedia
}

type SendQuotationInput struct {
	Channel            domain.QuotationSendChannel
	Mode               domain.QuotationSendMode
	Recipient          string // email saja; "" = email kontak
	Message            string // pesan pembuka; "" = default
	MailboxID          string
	SessionID          string
	ClientRequestID    string
	UserID             string
	CanReadAllWhatsApp bool
}

type SendQuotationResult struct {
	Quotation domain.Quotation
	Send      domain.QuotationSend
}

type QuotationPDF struct {
	Filename string
	Content  []byte
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
	// Revise membuat draft {nomor akar}-R{n} dari quotation sent/rejected/expired.
	Revise(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.Quotation, error)
	// PDF: draft dirender on-demand (watermark); selain draft dilayani dari
	// snapshot final (dibuat bila belum ada).
	PDF(ctx context.Context, scope coretenant.Scope, id string) (QuotationPDF, error)
	// MarkSentManually: snapshot PDF final + status sent + activity deal.
	MarkSentManually(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.Quotation, error)
	// SendVia mengirim lewat email/WhatsApp, mencatat log, dan menandai sent
	// pada kiriman sukses pertama. Kanal tidak tersedia → ChannelError (tidak dicatat).
	SendVia(ctx context.Context, scope coretenant.Scope, id string, in SendQuotationInput) (SendQuotationResult, error)
	Summary(ctx context.Context, scope coretenant.Scope, id, opening string) (quotationpdf.Summary, error)
	ListSends(ctx context.Context, scope coretenant.Scope, id string) ([]domain.QuotationSend, error)
}
