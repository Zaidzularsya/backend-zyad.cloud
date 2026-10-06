package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

// SelfServeCheckoutRequest: pembeli (tenant customer) memilih satu produk katalog platform.
type SelfServeCheckoutRequest struct {
	TenantOrganizationID string
	BuyerUserID          string
	ProductID            string
}

type SelfServeCheckoutResult struct {
	InvoiceURL string
	DealID     string
}

// SelfServeService menjalankan rantai company → lead → deal → quote → SO → invoice di CRM org
// platform. Setiap langkah cari-dulu-baru-buat sehingga memanggil ulang melanjutkan rantai yang
// terhenti tanpa menggandakan record.
type SelfServeService interface {
	Checkout(ctx context.Context, platform coretenant.Scope, req SelfServeCheckoutRequest) (SelfServeCheckoutResult, error)
}

// BuyerDirectory: data pembeli dan workspace-nya (diimplementasikan di internal/app).
type BuyerDirectory interface {
	Buyer(ctx context.Context, userID string) (name, email, phone string, err error)
	WorkspaceName(ctx context.Context, organizationID string) (string, error)
}

type SubscriptionLookup interface {
	// HasActiveFeatureContract: true bila company punya SO dengan contract aktif yang memuat produk berfitur.
	HasActiveFeatureContract(ctx context.Context, scope coretenant.Scope, companyID string) (bool, error)
}

// Locker: kunci eksklusif per kunci string (advisory lock Postgres).
type Locker interface {
	TryLock(ctx context.Context, key string) (unlock func(), ok bool, err error)
}

type SelfServeConfig struct{ BotUserID, DealOwnerUserID, PipelineID string }

// Subset service/repository CRM yang dipakai orkestrator (memudahkan fake di test).
type SelfServeLeads interface {
	Create(context.Context, coretenant.Scope, repository.CreateLeadParams) (domain.Lead, error)
	Convert(ctx context.Context, scope coretenant.Scope, id string, params ConvertLeadParams) (domain.LeadConversionResult, error)
	Delete(ctx context.Context, scope coretenant.Scope, id, deletedBy string) error
}

type SelfServeCompanies interface {
	FindByTenantOrganization(ctx context.Context, scope coretenant.Scope, tenantOrgID string) (domain.Company, error)
}

type SelfServeContacts interface {
	List(context.Context, coretenant.Scope, repository.ContactListFilter) ([]domain.Contact, int64, error)
}

type SelfServeDeals interface {
	ListOpenByCompanyAndPipeline(ctx context.Context, scope coretenant.Scope, companyID, pipelineID string) ([]domain.Deal, error)
	Update(ctx context.Context, scope coretenant.Scope, id string, params repository.UpdateDealParams) (domain.Deal, error)
	MoveStage(ctx context.Context, scope coretenant.Scope, id, stageID, updatedBy string) (domain.Deal, error)
	CloseLost(ctx context.Context, scope coretenant.Scope, id, lostReason, updatedBy string) (domain.Deal, error)
}

type SelfServePipelines interface {
	FindByID(context.Context, coretenant.Scope, string) (domain.Pipeline, error)
}

type SelfServeQuotations interface {
	Create(context.Context, coretenant.Scope, CreateQuotationInput) (domain.Quotation, error)
	FindSelfServeByDeal(ctx context.Context, scope coretenant.Scope, dealID string) (domain.Quotation, error)
	AcceptOnline(ctx context.Context, scope coretenant.Scope, id, actorUserID string) (domain.Quotation, error)
}

type SelfServeOrders interface {
	QuotationApproved(ctx context.Context, scope coretenant.Scope, q domain.Quotation, actorUserID string) error
	ListByDeal(ctx context.Context, scope coretenant.Scope, dealID string) ([]domain.SalesOrder, error)
	UpdateDraft(ctx context.Context, scope coretenant.Scope, id string, f repository.SalesOrderDraftFields, userID string) (domain.SalesOrder, error)
	Confirm(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.SalesOrder, error)
	RetryBilling(ctx context.Context, scope coretenant.Scope, id, userID string) (domain.SalesOrder, error)
	CancelUnpaid(ctx context.Context, scope coretenant.Scope, id, reason, actorUserID string) (domain.SalesOrder, error)
}

type SelfServeInvoices interface {
	Link(ctx context.Context, scope coretenant.Scope, id, userID string) (url string, expiresAt time.Time, err error)
}

type SelfServeDeps struct {
	Leads         SelfServeLeads
	Companies     SelfServeCompanies
	Contacts      SelfServeContacts
	Deals         SelfServeDeals
	Pipelines     SelfServePipelines
	Quotations    SelfServeQuotations
	Orders        SelfServeOrders
	Invoices      SelfServeInvoices
	Products      CatalogProducts
	Buyers        BuyerDirectory
	Subscriptions SubscriptionLookup
	Locker        Locker
}

var (
	ErrSelfServeProductUnavailable = errors.New("product is not available for self-serve checkout")
	ErrSelfServeAlreadySubscribed  = errors.New("workspace already has an active subscription")
	ErrSelfServeInProgress         = errors.New("another checkout for this workspace is in progress")
	// ErrSelfServeBillingNotReady: SO terkonfirmasi tetapi invoice awal belum terbit (penagihan gagal/menunggu).
	ErrSelfServeBillingNotReady = errors.New("initial invoice is not ready")
	// ErrSelfServeNotConfigured: bot/pipeline Self-Serve belum di-seed.
	ErrSelfServeNotConfigured = errors.New("self-serve checkout is not configured")
)

// Nama langkah untuk SelfServeStepError.Step.
const (
	SelfServeStepCompany    = "company"
	SelfServeStepLead       = "lead"
	SelfServeStepDeal       = "deal"
	SelfServeStepQuotation  = "quotation"
	SelfServeStepSalesOrder = "sales_order"
	SelfServeStepInvoice    = "invoice"
)

// SelfServeStepError membungkus kegagalan satu langkah rantai checkout.
type SelfServeStepError struct {
	Step string
	Err  error
}

func (e *SelfServeStepError) Error() string {
	return fmt.Sprintf("self-serve checkout step %s failed: %v", e.Step, e.Err)
}

func (e *SelfServeStepError) Unwrap() error { return e.Err }
