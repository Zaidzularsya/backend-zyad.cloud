package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	crmservice "zyad.cloud/internal/modules/crm/service"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	userrepo "zyad.cloud/internal/modules/user/repository"
	"zyad.cloud/internal/platform/database"
)

// selfServeBuyers: data pembeli (user) dan nama workspace-nya (organisasi tenant).
type selfServeBuyers struct {
	users *userrepo.UserRepository
	orgs  *organizationrepo.OrganizationRepository
}

func (b selfServeBuyers) Buyer(ctx context.Context, userID string) (string, string, string, error) {
	u, err := b.users.FindUserDetail(ctx, userID, false)
	if err != nil {
		return "", "", "", err
	}
	return u.Name, u.Email, u.Phone, nil
}

func (b selfServeBuyers) WorkspaceName(ctx context.Context, organizationID string) (string, error) {
	org, err := b.orgs.FindByID(ctx, organizationID)
	if err != nil {
		return "", err
	}
	return org.Name, nil
}

// selfServeSubscriptions: company dianggap sudah berlangganan bila punya SO confirmed/completed dengan
// contract aktif yang memuat produk berfitur.
type selfServeSubscriptions struct {
	orders    crmrepo.SalesOrderRepository
	contracts crmReceivableReader
	products  crmservice.CatalogProducts
}

func (s selfServeSubscriptions) HasActiveFeatureContract(ctx context.Context, scope coretenant.Scope, companyID string) (bool, error) {
	list, _, err := s.orders.List(ctx, scope, crmrepo.SalesOrderListFilter{CompanyID: companyID, Limit: 200})
	if err != nil {
		return false, err
	}
	for _, so := range list {
		if (so.Status != crmdomain.SalesOrderConfirmed && so.Status != crmdomain.SalesOrderCompleted) || so.ContractID == "" {
			continue
		}
		status, _, err := s.contracts.ContractInfo(ctx, scope, so.ContractID)
		if err != nil {
			return false, err
		}
		if status != string(receivabledomain.ContractActive) {
			continue
		}
		full, err := s.orders.FindByID(ctx, scope, so.ID) // List tidak memuat item
		if err != nil {
			return false, err
		}
		var ids []string
		for _, it := range full.Items {
			if it.ProductID != "" {
				ids = append(ids, it.ProductID)
			}
		}
		if len(ids) == 0 {
			continue
		}
		products, err := s.products.FindByIDs(ctx, scope, ids)
		if err != nil {
			return false, err
		}
		for _, p := range products {
			if len(p.Features) > 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

// ---- receivable → CRM: pembatalan SO yang belum dibayar ----

func (r crmReceivableReader) InvoiceState(ctx context.Context, scope coretenant.Scope, id string) (string, bool, error) {
	inv, err := r.invoices.Get(ctx, scope, id)
	if err != nil {
		return "", false, err
	}
	paid, ok := new(big.Rat).SetString(inv.AmountPaid)
	return string(inv.Status), inv.Status == receivabledomain.InvoicePaid || (ok && paid.Sign() > 0), nil
}

func (r crmReceivableReader) VoidInvoice(ctx context.Context, scope coretenant.Scope, id, reason, userID string) error {
	_, err := r.invoices.Void(ctx, scope, id, reason, userID)
	return err
}

// EndContract mengakhiri contract hari ini; no-op bila contract sudah tidak aktif.
func (r crmReceivableReader) EndContract(ctx context.Context, scope coretenant.Scope, id, reason, userID string) error {
	c, err := r.contracts.Get(ctx, scope, id)
	if err != nil {
		return err
	}
	if c.Status != receivabledomain.ContractActive {
		return nil
	}
	_, err = r.contracts.End(ctx, scope, id, time.Time{}, reason, userID)
	return err
}

// selfServeDeals menggabungkan query deal (repository) dan transisi deal (service).
type selfServeDeals struct {
	repo crmrepo.DealRepository
	svc  crmservice.DealService
}

func (d selfServeDeals) ListOpenByCompanyAndPipeline(ctx context.Context, scope coretenant.Scope, companyID, pipelineID string) ([]crmdomain.Deal, error) {
	return d.repo.ListOpenByCompanyAndPipeline(ctx, scope, companyID, pipelineID)
}
func (d selfServeDeals) Update(ctx context.Context, scope coretenant.Scope, id string, p crmrepo.UpdateDealParams) (crmdomain.Deal, error) {
	return d.svc.Update(ctx, scope, id, p)
}
func (d selfServeDeals) MoveStage(ctx context.Context, scope coretenant.Scope, id, stageID, by string) (crmdomain.Deal, error) {
	return d.svc.MoveStage(ctx, scope, id, stageID, by)
}
func (d selfServeDeals) CloseLost(ctx context.Context, scope coretenant.Scope, id, reason, by string) (crmdomain.Deal, error) {
	return d.svc.CloseLost(ctx, scope, id, reason, by)
}

// ---- konfigurasi runtime ----

// selfServeResolver membangun SelfServeService saat pertama dibutuhkan. Bot, pemilik deal, dan pipeline
// Self-Serve dibuat oleh seeder; bila belum ada, API tetap hidup dan endpoint membalas 503 sampai seeder
// dijalankan (tanpa perlu restart).
type selfServeResolver struct {
	db       *database.Pool
	cfg      selfServeRuntimeConfig
	pipeline crmrepo.PipelineRepository
	build    func(crmservice.SelfServeConfig) crmservice.SelfServeService

	mu    sync.Mutex
	inner crmservice.SelfServeService
}

type selfServeRuntimeConfig struct{ BotEmail, DealOwnerEmail string }

func (r *selfServeResolver) Checkout(ctx context.Context, platform coretenant.Scope, req crmservice.SelfServeCheckoutRequest) (crmservice.SelfServeCheckoutResult, error) {
	svc, err := r.service(ctx, platform)
	if err != nil {
		slog.ErrorContext(ctx, "self-serve is not configured", "error", err)
		return crmservice.SelfServeCheckoutResult{}, crmservice.ErrSelfServeNotConfigured
	}
	return svc.Checkout(ctx, platform, req)
}

func (r *selfServeResolver) service(ctx context.Context, platform coretenant.Scope) (crmservice.SelfServeService, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inner != nil {
		return r.inner, nil
	}
	botID, err := r.userIDByEmail(ctx, r.cfg.BotEmail)
	if err != nil {
		return nil, fmt.Errorf("self-serve bot %q: %w (jalankan: cmd/seed -name self-serve)", r.cfg.BotEmail, err)
	}
	ownerID := botID
	if email := strings.TrimSpace(r.cfg.DealOwnerEmail); email != "" {
		if ownerID, err = r.userIDByEmail(ctx, email); err != nil {
			return nil, fmt.Errorf("self-serve deal owner %q: %w", email, err)
		}
	}
	pipeline, err := r.pipeline.FindBySystemKey(ctx, platform, "self_serve")
	if err != nil {
		return nil, fmt.Errorf("self-serve pipeline: %w (jalankan: cmd/seed -name self-serve)", err)
	}
	r.inner = r.build(crmservice.SelfServeConfig{BotUserID: botID, DealOwnerUserID: ownerID, PipelineID: pipeline.ID})
	return r.inner, nil
}

func (r *selfServeResolver) userIDByEmail(ctx context.Context, email string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL`, strings.TrimSpace(email)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("user not found")
	}
	return id, err
}

var (
	_ crmservice.OrderBillingCanceller = crmReceivableReader{}
	_ crmservice.SubscriptionLookup    = selfServeSubscriptions{}
	_ crmservice.BuyerDirectory        = selfServeBuyers{}
)
