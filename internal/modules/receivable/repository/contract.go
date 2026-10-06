package repository

import (
	"context"
	"errors"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/shared/pricing"
)

var (
	ErrInvoiceNotDraft        = errors.New("invoice is not draft")
	ErrInvoiceNotPayable      = errors.New("invoice is not issued or overdue")
	ErrInvoiceHasPayments     = errors.New("invoice already has payments")
	ErrPaymentExceeds         = errors.New("payment exceeds remaining balance")
	ErrDuplicateSource        = errors.New("invoice for this source and idempotency key already exists")
	ErrDuplicateAccountSource = errors.New("account for this source already exists")
)

// Semua method ber-scope: organisasi diambil dari scope dan setiap query
// memfilter organization_id secara eksplisit (selain RLS).

type AccountParams struct{ Name, CompanyName, Email, Phone, Address, SourceType, SourceID string }

type AccountRepository interface {
	// Create: sumber (source_type, source_id) yang sudah punya account → ErrDuplicateAccountSource.
	Create(ctx context.Context, scope coretenant.Scope, p AccountParams, createdBy string) (domain.Account, error)
	// Update mengubah nama/kontak; sumber account tidak berubah.
	Update(ctx context.Context, scope coretenant.Scope, id string, p AccountParams) (domain.Account, error)
	// FindByID / FindBySource mengembalikan pgx.ErrNoRows bila tidak ada.
	FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Account, error)
	FindBySource(ctx context.Context, scope coretenant.Scope, sourceType, sourceID string) (domain.Account, error)
	List(ctx context.Context, scope coretenant.Scope, search string, limit, offset int) ([]domain.Account, int64, error)
}

type InvoiceItemParams struct {
	Line                                               pricing.PricedLine
	Unit, ProductID, SKU, ContractItemID, SourceLineID string
	PeriodStart, PeriodEnd                             *time.Time
}

type CreateInvoiceParams struct {
	AccountID, SourceType, SourceID, ContractID, IdempotencyKey, Currency, Notes, PICUserID, CreatedBy string
	Channels                                                                                           []string
	PeriodStart, PeriodEnd                                                                             *time.Time
	Totals                                                                                             pricing.Totals
	Items                                                                                              []InvoiceItemParams
}

type InvoiceListFilter struct {
	Status, AccountID, SourceType, SourceID, ContractID string
	// SendFailed: ada kiriman gagal tanpa kiriman sukses sesudahnya pada kanal yang sama.
	SendFailed bool
	Search     string // nomor invoice atau nama/perusahaan account
	Limit      int
	Offset     int
}

type InvoiceRepository interface {
	// Create membuat draft. Unik source+idempotency_key → ErrDuplicateSource (pemanggil membaca FindBySource).
	Create(ctx context.Context, scope coretenant.Scope, p CreateInvoiceParams) (domain.Invoice, error)
	// FindByID / FindBySource mengembalikan pgx.ErrNoRows bila tidak ada.
	FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Invoice, error)
	FindBySource(ctx context.Context, scope coretenant.Scope, sourceType, sourceID, idempotencyKey string) (domain.Invoice, error)
	// List tidak memuat Items.
	List(ctx context.Context, scope coretenant.Scope, f InvoiceListFilter) ([]domain.Invoice, int64, error)
	ReplaceDraft(ctx context.Context, scope coretenant.Scope, id string, p CreateInvoiceParams) (domain.Invoice, error) // ErrInvoiceNotDraft
	// Issue: draft → issued dengan nomor & tanggal; kondisional status = 'draft' (ErrInvoiceNotDraft).
	Issue(ctx context.Context, scope coretenant.Scope, id, number string, issueDate, dueDate time.Time, by string) (domain.Invoice, error)
	SetPDF(ctx context.Context, scope coretenant.Scope, id, assetID string) error
	// Void: ErrInvoiceHasPayments bila sudah ada pembayaran; ErrInvoiceNotPayable bila sudah void.
	Void(ctx context.Context, scope coretenant.Scope, id, reason, by string) (domain.Invoice, error)
	// MarkOverdue: issued AND due_date < today → overdue. Mengembalikan jumlah baris.
	MarkOverdue(ctx context.Context, scope coretenant.Scope, today time.Time) (int64, error)
}

type PaymentParams struct {
	Amount, Method, Reference, Note, RecordedBy string
	PaidAt                                      time.Time
	ProviderPayload                             map[string]any
}

type PaymentRepository interface {
	// Record: SELECT invoice FOR UPDATE; status issued|overdue; amount <= sisa; amount_paid += amount;
	// lunas → status paid + paid_at. Satu transaksi. (method, reference) provider duplikat → pembayaran lama
	// dikembalikan (created=false) dan invoice tidak berubah. Referensi method manual tidak idempoten.
	Record(ctx context.Context, scope coretenant.Scope, invoiceID string, p PaymentParams) (domain.Invoice, domain.Payment, bool, error)
	List(ctx context.Context, scope coretenant.Scope, invoiceID string) ([]domain.Payment, error)
	// ListAll terbaru dulu, dengan nomor invoice & nama account.
	ListAll(ctx context.Context, scope coretenant.Scope, limit, offset int) ([]domain.Payment, int64, error)
}

type RecordSendParams struct {
	InvoiceID, Channel, Recipient, Status, Error, ExternalMessageID, ClientRequestID, Trigger, SentBy string
}

// SendRepository mencatat log kiriman invoice. client_request_id unik per invoice (idempotensi kirim).
type SendRepository interface {
	Record(ctx context.Context, scope coretenant.Scope, p RecordSendParams) (domain.Send, error)
	// FindByClientRequest dan LatestFailedByInvoice mengembalikan pgx.ErrNoRows bila tidak ada.
	FindByClientRequest(ctx context.Context, scope coretenant.Scope, invoiceID, clientRequestID string) (domain.Send, error)
	// ListByInvoice: terbaru dulu.
	ListByInvoice(ctx context.Context, scope coretenant.Scope, invoiceID string) ([]domain.Send, error)
	// LatestFailedByInvoice: kiriman gagal terbaru yang belum disusul kiriman sukses pada kanal yang sama.
	LatestFailedByInvoice(ctx context.Context, scope coretenant.Scope, invoiceID string) (domain.Send, error)
	// LatestByInvoices: kiriman terbaru per invoice dalam satu query; invoice tanpa kiriman tidak muncul di peta.
	LatestByInvoices(ctx context.Context, scope coretenant.Scope, invoiceIDs []string) (map[string]domain.Send, error)
}

type SettingsRepository interface {
	// Get tanpa baris → default (7, 7, [email], "").
	Get(ctx context.Context, scope coretenant.Scope) (domain.Settings, error)
	Upsert(ctx context.Context, scope coretenant.Scope, s domain.Settings, by string) (domain.Settings, error)
}

type CounterRepository interface {
	// Next: INSERT … ON CONFLICT DO UPDATE last_number+1 RETURNING (atomik, per org/tipe/tahun).
	Next(ctx context.Context, scope coretenant.Scope, documentType string, year int) (int, error)
}
