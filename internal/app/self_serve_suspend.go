package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/core/businesstime"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	notificationrepo "zyad.cloud/internal/core/notification/repository"
	coretenant "zyad.cloud/internal/core/tenant"
	crmservice "zyad.cloud/internal/modules/crm/service"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivablerepo "zyad.cloud/internal/modules/receivable/repository"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/platform/database"
)

// withBillingSuspend mengaktifkan suspend/aktif kembali karena tagihan pada TenantAccess.
func withBillingSuspend(access *crmservice.TenantAccess, cfg config.Config, db *database.Pool, invoices receivableservice.InvoiceService) *crmservice.TenantAccess {
	orgs := organizationrepo.NewOrganizationRepository(db)
	lifecycle := organizationservice.NewLifecycleService(organizationrepo.NewLifecycleRepository(db))
	notifier := accessNotifier{
		owners: workspaceOwnerRepo{db: db}, orgs: orgs,
		publisher: notificationpublisher.NewOutboxPublisher(notificationrepo.NewOutboxRepository(db), cfg.Notification.MaxAttempts),
		appName:   cfg.App.Name, locale: cfg.Notification.DefaultLocale,
	}
	bot := &botUser{db: db, email: cfg.SelfServe.BotEmail}
	return access.WithSuspend(billingInvoices{invoices: invoices}, newWorkspaceStatus(orgs, lifecycle), notifier, bot.ID)
}

// billingInvoices mengadaptasi InvoiceService ke crmservice.BillingInvoiceReader.
type billingInvoices struct {
	invoices receivableservice.InvoiceService
}

var _ crmservice.BillingInvoiceReader = billingInvoices{}

func (b billingInvoices) OverdueDueBefore(ctx context.Context, scope coretenant.Scope, before time.Time) ([]crmservice.BillingInvoice, error) {
	const pageSize = 200
	var out []crmservice.BillingInvoice
	for offset := 0; ; offset += pageSize {
		page, total, err := b.invoices.List(ctx, scope, receivablerepo.InvoiceListFilter{
			Status: string(receivabledomain.InvoiceOverdue), DueBefore: &before, Limit: pageSize, Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		for _, inv := range page {
			out = append(out, billingInvoice(inv))
		}
		if int64(offset+len(page)) >= total || len(page) == 0 {
			return out, nil
		}
	}
}

func (b billingInvoices) Invoice(ctx context.Context, scope coretenant.Scope, id string) (crmservice.BillingInvoice, error) {
	inv, err := b.invoices.Get(ctx, scope, id)
	if err != nil {
		return crmservice.BillingInvoice{}, err
	}
	return billingInvoice(inv), nil
}

func (b billingInvoices) Link(ctx context.Context, scope coretenant.Scope, id, actorUserID string) (string, error) {
	url, _, err := b.invoices.Link(ctx, scope, id, actorUserID)
	return url, err
}

func billingInvoice(inv receivabledomain.Invoice) crmservice.BillingInvoice {
	return crmservice.BillingInvoice{
		ID: inv.ID, Number: inv.InvoiceNumber, Status: string(inv.Status), SourceType: string(inv.SourceType),
		SourceID: inv.SourceID, ContractID: inv.ContractID, DueDate: inv.DueDate, AmountDue: inv.Balance(),
	}
}

// botUser memetakan email bot self-serve ke user id (aktor audit perubahan status), di-cache setelah berhasil.
type botUser struct {
	db    *database.Pool
	email string

	mu sync.Mutex
	id string
}

func (b *botUser) ID(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.id != "" {
		return b.id, nil
	}
	var id string
	err := b.db.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL`, strings.TrimSpace(b.email)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("self-serve bot %q not found (jalankan: cmd/seed -name self-serve)", b.email)
	}
	if err != nil {
		return "", err
	}
	b.id = id
	return id, nil
}

// SelfServeSuspendRunner menjalankan SuspendOverdue per jam (cmd/worker).
type SelfServeSuspendRunner struct {
	access    *crmservice.TenantAccess
	scopes    *platformScopeResolver
	graceDays int
	now       func() time.Time
}

func NewSelfServeSuspendRunner(cfg config.Config, db *database.Pool, log *slog.Logger) (*SelfServeSuspendRunner, error) {
	if cfg.SelfServe.GraceDaysInvalid {
		log.Warn("SELF_SERVE_GRACE_DAYS di luar rentang 0-60 atau bukan angka; memakai default", "grace_days", cfg.SelfServe.GraceDays)
	}
	module, err := newWorkerReceivableModule(cfg, db, log)
	if err != nil {
		return nil, err
	}
	access := withBillingSuspend(newCRMTenantAccess(db, module.Invoices, module.Contracts, workerDefaultAccess(cfg, db)), cfg, db, module.Invoices)
	return &SelfServeSuspendRunner{
		access: access, scopes: newPlatformScopeResolver(organizationrepo.NewOrganizationRepository(db)),
		graceDays: cfg.SelfServe.GraceDays, now: time.Now,
	}, nil
}

func (r *SelfServeSuspendRunner) RunOnce(ctx context.Context) (crmservice.SuspendResult, error) {
	scope, err := r.scopes.PlatformScope(ctx)
	if err != nil {
		return crmservice.SuspendResult{}, fmt.Errorf("platform scope: %w", err)
	}
	return r.access.SuspendOverdue(ctx, scope, businesstime.DayOf(r.now()), r.graceDays)
}
