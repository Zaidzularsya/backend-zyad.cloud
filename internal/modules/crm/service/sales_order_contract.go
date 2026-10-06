package service

import (
	"context"
	"errors"
	"strings"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

var (
	ErrBatchKeyRequired        = errors.New("batch_key is required")
	ErrNothingToDeliver        = errors.New("select at least one item to confirm")
	ErrDeliveryDateInFuture    = errors.New("delivery date must not be in the future")
	ErrSalesOrderNotConfirmed  = errors.New("sales order is not confirmed")
	ErrBillingNotRetryable     = errors.New("billing is not awaiting retry")
	ErrInvalidSalesOrderFields = errors.New("invalid sales order fields")
)

// IncompleteError: SO belum memenuhi syarat konfirmasi; dipetakan ke 422 SALES_ORDER_INCOMPLETE {fields}.
type IncompleteError struct{ Fields []string }

func (e *IncompleteError) Error() string { return "data SO belum lengkap" }

type DeliveryInput struct {
	ItemIDs     []string
	DeliveredAt time.Time
	Note        string
	BatchKey    string // UUID dari klien; satu batch = satu invoice (idempoten)
}

type SalesOrderService interface {
	// QuotationApproved: implementasi hook approve (S2). Idempoten per quotation.
	QuotationApproved(ctx context.Context, scope coretenant.Scope, q domain.Quotation, actorUserID string) error
	Get(ctx context.Context, scope coretenant.Scope, id string) (domain.SalesOrder, error)
	List(ctx context.Context, scope coretenant.Scope, f repository.SalesOrderListFilter) ([]domain.SalesOrder, int64, error)
	ListByDeal(ctx context.Context, scope coretenant.Scope, dealID string) ([]domain.SalesOrder, error)
	UpdateDraft(ctx context.Context, scope coretenant.Scope, id string, f repository.SalesOrderDraftFields, userID string) (domain.SalesOrder, error)
	// Confirm: kegagalan penagihan TIDAK menjadi error; SO dikembalikan dengan billing_status failed.
	Confirm(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.SalesOrder, error)
	// RetryBilling: SO confirmed/completed dengan billing_status failed|pending.
	RetryBilling(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.SalesOrder, error)
	ConfirmDelivery(ctx context.Context, scope coretenant.Scope, id string, in DeliveryInput, userID string) (domain.SalesOrder, error)
	Cancel(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.SalesOrder, error)
}

// DealWonEvaluator dipicu setelah konfirmasi SO, konfirmasi diterima, dan pembayaran/kontrak (lihat WonEvaluator).
type DealWonEvaluator interface {
	Evaluate(ctx context.Context, scope coretenant.Scope, dealID string) (bool, error)
}

// InvoiceStatusReader / ContractReader: pembacaan sempit ke modul receivable (diadaptasi di internal/app).
type InvoiceStatusReader interface {
	InvoiceStatus(ctx context.Context, scope coretenant.Scope, id string) (status, number string, err error)
}

type ContractReader interface {
	ContractNumber(ctx context.Context, scope coretenant.Scope, id string) (string, error)
}

// SalesOrderDeps: dependensi non-inti service sales order. Reader/Notifier boleh nil (dilewati).
type SalesOrderDeps struct {
	Counters    repository.DocumentCounterRepository
	Contacts    repository.ContactRepository
	Companies   repository.CompanyRepository
	Deals       repository.DealRepository
	Members     repository.MemberRepository
	Activities  repository.ActivityRepository
	Invoices    InvoiceStatusReader
	Contracts   ContractReader
	Notifier    NotificationPublisher
	AppName     string
	FrontendURL string
	Locale      string
}

func (d SalesOrderDeps) soURL(id string) string {
	return strings.TrimRight(d.FrontendURL, "/") + "/app/sales/orders/" + id
}

// ContractInfoReader: status & nomor kontrak untuk tautan di respons SO (diadaptasi di internal/app).
type ContractInfoReader interface {
	ContractInfo(ctx context.Context, scope coretenant.Scope, id string) (status, number string, err error)
}
