package service

import (
	"context"
	"errors"
	"io"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/shared/docpdf"
	"zyad.cloud/internal/shared/pricing"
	"zyad.cloud/internal/shared/publiclink"
)

var (
	// ErrInvalidInvoice membungkus detail validasi (errors.Is(err, ErrInvalidInvoice)).
	ErrInvalidInvoice       = errors.New("invalid invoice")
	ErrInvalidAccount       = errors.New("invalid account")
	ErrInvalidPayment       = errors.New("amount must be > 0 with at most 2 decimals and paid_at must not be in the future")
	ErrVoidReasonRequired   = errors.New("void reason is required (1-500 characters)")
	ErrInvoiceNotSendable   = errors.New("invoice can only be sent when issued, overdue or paid")
	ErrDocumentsUnavailable = errors.New("invoice documents are not configured")
)

// FileStore: subset asset service untuk snapshot PDF privat.
type FileStore interface {
	UploadObject(ctx context.Context, scope coretenant.Scope, params assetservice.UploadObjectParams, content io.Reader) (assetdomain.AssetObject, error)
	OpenObject(ctx context.Context, scope coretenant.Scope, id string) (io.ReadCloser, assetdomain.AssetObject, error)
	DeleteObject(ctx context.Context, scope coretenant.Scope, id string) error
}

type PDFRenderer interface {
	Render(docpdf.Document) ([]byte, error)
}

// IssuerSource memberi identitas penerbit (organisasi tenant) untuk PDF.
type IssuerSource interface {
	OrganizationProfile(ctx context.Context, scope coretenant.Scope) (docpdf.Party, error)
}

// Links adalah subset publiclink.Service yang dipakai invoice.
type Links interface {
	Ensure(ctx context.Context, scope coretenant.Scope, docType, docID string, expiresAt time.Time, createdBy string) (publiclink.Link, error)
	Revoke(ctx context.Context, scope coretenant.Scope, docType, docID string) error
}

// AutoSender mengirim invoice yang baru terbit ke semua kanalnya (diimplementasikan SendService, Task 5).
type AutoSender interface {
	AutoSend(ctx context.Context, scope coretenant.Scope, inv domain.Invoice, actorUserID string)
}

type LineInput struct {
	pricing.LineInput
	Unit, ProductID, SKU, ContractItemID, SourceLineID string
	PeriodStart, PeriodEnd                             *time.Time
}

type InvoiceInput struct {
	AccountID                                                        string
	SourceType                                                       domain.SourceType // "" → manual
	SourceID, ContractID, IdempotencyKey, Currency, Notes, PICUserID string
	Channels                                                         []string // kosong → settings.DefaultChannels
	PeriodStart, PeriodEnd                                           *time.Time
	Lines                                                            []LineInput
}

type InvoicePDF struct {
	Filename string
	Content  []byte
}

type InvoiceService interface {
	CreateDraft(ctx context.Context, scope coretenant.Scope, in InvoiceInput, userID string) (domain.Invoice, error)
	UpdateDraft(ctx context.Context, scope coretenant.Scope, id string, in InvoiceInput, userID string) (domain.Invoice, error)
	// CreateAndIssue: idempoten by (SourceType, SourceID, IdempotencyKey) — duplikat → kembalikan invoice lama
	// tanpa kirim ulang. Mengirim otomatis (trigger "auto") setelah terbit. Dipakai S4/S5.
	CreateAndIssue(ctx context.Context, scope coretenant.Scope, in InvoiceInput, actorUserID string) (domain.Invoice, error)
	// Issue: terbit + kirim otomatis; idempoten (sudah issued → kembalikan apa adanya, tanpa kirim).
	Issue(ctx context.Context, scope coretenant.Scope, id, actorUserID string) (domain.Invoice, error)
	Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Invoice, error)
	List(ctx context.Context, scope coretenant.Scope, f repository.InvoiceListFilter) ([]domain.Invoice, int64, error)
	Void(ctx context.Context, scope coretenant.Scope, id, reason, userID string) (domain.Invoice, error)
	// PDF: draft dirender on-demand dengan Stamp DRAFT; selain itu snapshot (dibuat bila belum ada).
	PDF(ctx context.Context, scope coretenant.Scope, id string) (InvoicePDF, error)
	// Link: status issued|overdue|paid.
	Link(ctx context.Context, scope coretenant.Scope, id, userID string) (url string, expiresAt time.Time, err error)
	// RefreshSnapshot membuat ulang snapshot PDF (stempel berubah, mis. LUNAS); best effort bagi pemanggil.
	RefreshSnapshot(ctx context.Context, scope coretenant.Scope, id string) error
}

type InvoiceDeps struct {
	Accounts    repository.AccountRepository
	Invoices    repository.InvoiceRepository
	Counters    repository.CounterRepository
	Settings    repository.SettingsRepository
	Issuers     IssuerSource
	Files       FileStore
	Renderer    PDFRenderer
	Links       Links
	Sender      AutoSender // nil = tanpa kirim otomatis
	Listeners   *Registry
	FrontendURL string
	Now         func() time.Time // nil = time.Now
}

type ManualPaymentInput struct {
	Amount          string
	PaidAt          time.Time // zero = sekarang
	Reference, Note string
}

type PaymentService interface {
	RecordManual(ctx context.Context, scope coretenant.Scope, invoiceID string, in ManualPaymentInput, userID string) (domain.Invoice, domain.Payment, error)
	// RecordProvider: dipakai S6 (DOKU). Idempoten by reference.
	RecordProvider(ctx context.Context, scope coretenant.Scope, invoiceID string, p repository.PaymentParams) (domain.Invoice, error)
	List(ctx context.Context, scope coretenant.Scope, invoiceID string) ([]domain.Payment, error)
	ListAll(ctx context.Context, scope coretenant.Scope, limit, offset int) ([]domain.Payment, int64, error)
}

type PaymentDeps struct {
	Payments  repository.PaymentRepository
	Invoices  InvoiceService // dipakai untuk regenerasi snapshot setelah lunas
	Listeners *Registry
	Now       func() time.Time
}

type AccountInput struct{ Name, CompanyName, Email, Phone, Address, SourceType, SourceID string }

type AccountService interface {
	// Create: SourceType/SourceID hanya diisi pemanggil internal (CRM/S4); handler tidak membukanya.
	Create(ctx context.Context, scope coretenant.Scope, in AccountInput, userID string) (domain.Account, error)
	// EnsureForSource mengembalikan account untuk sumber itu, membuatnya bila belum ada (aman terhadap balapan).
	EnsureForSource(ctx context.Context, scope coretenant.Scope, in AccountInput, userID string) (domain.Account, error)
	Update(ctx context.Context, scope coretenant.Scope, id string, in AccountInput) (domain.Account, error)
	Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Account, error)
	List(ctx context.Context, scope coretenant.Scope, search string, limit, offset int) ([]domain.Account, int64, error)
}

// Listener: modul lain (CRM, S4) mendaftar; dipanggil SETELAH commit, error hanya di-log.
type InvoiceRef struct {
	ID, Number, ContractID, SourceID string
	SourceType                       domain.SourceType
}

type ContractRef struct {
	ID, Number, SourceID string
	SourceType           domain.SourceType
}

type Listener interface {
	InvoicePaid(ctx context.Context, scope coretenant.Scope, inv InvoiceRef)
	ContractCreated(ctx context.Context, scope coretenant.Scope, c ContractRef) // dipanggil mulai S4
}
