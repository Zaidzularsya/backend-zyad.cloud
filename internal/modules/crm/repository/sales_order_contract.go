package repository

import (
	"context"
	"errors"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

// ErrSalesOrderNotDraft / ErrDeliveryNotPending diekspos dari domain agar service tidak mengimpor repository.
var (
	ErrSalesOrderNotDraft = domain.ErrSalesOrderNotDraft
	ErrDeliveryNotPending = domain.ErrDeliveryNotPending
)

type SalesOrderDraftFields struct {
	StartDate                                                          *time.Time
	BillToName, BillToCompany, BillToEmail, BillToPhone, BillToAddress string
	Channels                                                           []string
	PICUserID                                                          string
}

type SalesOrderListFilter struct {
	Status, BillingStatus, DealID, CompanyID, Search string
	Limit, Offset                         int
}

type SalesOrderRepository interface {
	// CreateFromQuotation: unik quotation_id → kembalikan SO yang ada (created=false).
	CreateFromQuotation(ctx context.Context, scope coretenant.Scope, q domain.Quotation, number string, defaults SalesOrderDraftFields, createdBy string) (domain.SalesOrder, bool, error)
	// FindByID / FindByQuotation mengembalikan pgx.ErrNoRows bila tidak ada.
	FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.SalesOrder, error)
	FindByQuotation(ctx context.Context, scope coretenant.Scope, quotationID string) (domain.SalesOrder, error)
	ListByDeal(ctx context.Context, scope coretenant.Scope, dealID string) ([]domain.SalesOrder, error)
	// List tidak memuat Items.
	List(ctx context.Context, scope coretenant.Scope, f SalesOrderListFilter) ([]domain.SalesOrder, int64, error)
	UpdateDraft(ctx context.Context, scope coretenant.Scope, id string, f SalesOrderDraftFields, by string) (domain.SalesOrder, error) // ErrSalesOrderNotDraft
	// MarkConfirmed: draft → confirmed, billing_status pending; kondisional (ErrSalesOrderNotDraft).
	MarkConfirmed(ctx context.Context, scope coretenant.Scope, id, by string) (domain.SalesOrder, error)
	// SetBillingResult: status done → SO completed bila tak ada lagi yang menunggu (lihat MarkDelivered).
	SetBillingResult(ctx context.Context, scope coretenant.Scope, id string, status domain.BillingStatus, errMsg, accountID, initialInvoiceID, contractID string) (domain.SalesOrder, error)
	// MarkDelivered: item pending → delivered (+invoice_id); ErrDeliveryNotPending bila ada yang bukan pending;
	// SO → completed bila tak ada lagi item menunggu diterima dan semua baris one_time prabayar sudah ditagih.
	MarkDelivered(ctx context.Context, scope coretenant.Scope, id string, itemIDs []string, at time.Time, note, invoiceID, by string) (domain.SalesOrder, error)
	Cancel(ctx context.Context, scope coretenant.Scope, id, by string) (domain.SalesOrder, error) // draft saja
	// MarkCancelled: confirmed → cancelled; ErrSalesOrderNotConfirmed bila status lain.
	MarkCancelled(ctx context.Context, scope coretenant.Scope, id, by string) (domain.SalesOrder, error)
}

var ErrSalesOrderNotConfirmed = errors.New("sales order is not confirmed")
