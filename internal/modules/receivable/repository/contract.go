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
	ErrContractNotActive      = domain.ErrContractNotActive
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

type ContractItemParams struct {
	Description, Quantity, Unit, UnitPrice, DiscountPercent, TaxPercent, ProductID, SKU, SourceLineID string
	Frequency                                                                                         pricing.Frequency
	PaymentTiming                                                                                     pricing.PaymentTiming
	PeriodIndex                                                                                       int
	NextPeriodStart, NextPeriodEnd                                                                    time.Time
}

type CreateContractParams struct {
	Number, AccountID, SourceType, SourceID, Currency, Notes, PICUserID, CreatedBy string
	StartDate                                                                      time.Time
	Channels                                                                       []string
	Items                                                                          []ContractItemParams
}

type ContractListFilter struct {
	Status, AccountID, Search string
	Limit, Offset             int
}

type ContractRepository interface {
	// Create: unik (source_type, source_id) → ErrDuplicateSource (pemanggil membaca FindBySource).
	Create(ctx context.Context, scope coretenant.Scope, p CreateContractParams) (domain.Contract, error)
	// FindByID / FindBySource mengembalikan pgx.ErrNoRows bila tidak ada.
	FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Contract, error)
	FindBySource(ctx context.Context, scope coretenant.Scope, sourceType, sourceID string) (domain.Contract, error)
	// List tidak memuat Items.
	List(ctx context.Context, scope coretenant.Scope, f ContractListFilter) ([]domain.Contract, int64, error)
	// SetEndDate: hanya contract active (ErrContractNotActive); nil menghapus tanggal akhir.
	SetEndDate(ctx context.Context, scope coretenant.Scope, id string, endDate *time.Time, by string) (domain.Contract, error)
	// End: active → ended (ErrContractNotActive bila bukan active).
	End(ctx context.Context, scope coretenant.Scope, id string, endDate time.Time, reason, by string) (domain.Contract, error)
	// DueItems: item contract active yang BillOn <= today, urut contract_id, next_period_start.
	// BillOn: prabayar = next_period_start − leadDays; pascabayar = next_period_end + 1 hari.
	// Item yang periodenya mulai setelah end_date contract tidak ikut.
	DueItems(ctx context.Context, scope coretenant.Scope, today time.Time, leadDays, limit int) ([]DueItem, error)
	// Advance: period_index+1 dan next_period_start/end baru, hanya bila period_index masih = from (false bila sudah dimajukan).
	Advance(ctx context.Context, scope coretenant.Scope, itemID string, from int, nextStart, nextEnd time.Time) (bool, error)
	// EndExpired: active dengan end_date < today dan tanpa item yang next_period_start <= end_date → ended.
	EndExpired(ctx context.Context, scope coretenant.Scope, today time.Time) (int64, error)
}

// DueItem: satu item contract yang sudah waktunya ditagih. Contract adalah header tanpa Items.
type DueItem struct {
	ContractID, ContractNumber, AccountID, ItemID string
	Contract                                      domain.Contract
	Item                                          domain.ContractItem
	BillOn                                        time.Time
}

// ActiveItem: item contract aktif beserta identitas contract dan nama account (bahan monitor Recurring Billing).
type ActiveItem struct {
	ContractID, ContractNumber, AccountName string
	StartDate                               time.Time
	EndDate                                 *time.Time
	Item                                    domain.ContractItem
}

type UnpaidSummary struct {
	Count, OverdueCount int
	TotalBalance        string // Σ (grand_total − amount_paid) invoice issued|overdue
}

// FailedSend: kiriman gagal terakhir sebuah invoice yang belum disusul kiriman sukses pada kanal yang sama.
type FailedSend struct {
	InvoiceID, InvoiceNumber, AccountName, Channel, Error string
	SentAt                                                time.Time
}

// OverviewRepository: query baca-saja untuk halaman Recurring Billing.
type OverviewRepository interface {
	// ActiveItems: item semua contract active, urut contract_id lalu position; dibatasi limit.
	ActiveItems(ctx context.Context, scope coretenant.Scope, limit int) ([]ActiveItem, error)
	// Unpaid: invoice issued|overdue; overdue = status overdue atau due_date < today.
	Unpaid(ctx context.Context, scope coretenant.Scope, today time.Time) (UnpaidSummary, error)
	// FailedSends: terbaru dulu, satu baris per (invoice, kanal), maks limit.
	FailedSends(ctx context.Context, scope coretenant.Scope, limit int) ([]FailedSend, error)
}
