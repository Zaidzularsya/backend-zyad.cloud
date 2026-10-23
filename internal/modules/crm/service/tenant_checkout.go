package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
)

// TenantCheckoutInput: semua field berasal dari request publik, kecuali tenant (dari host).
type TenantCheckoutInput struct {
	GoogleCredential string
	ProductID        string
	IP               string
	UserAgent        string
}

// TenantCheckoutService menjalankan checkout mandiri pelanggan tenant dari domain tenant:
// verifikasi Google → user → workspace pembeli (satu per tenant-pembeli) → rantai CRM di scope
// tenant → tautan company↔workspace → link pembayaran invoice.
type TenantCheckoutService interface {
	Checkout(ctx context.Context, tenant coretenant.Scope, in TenantCheckoutInput) (SelfServeCheckoutResult, error)
}

// TenantBuyer: user hasil verifikasi kredensial Google.
type TenantBuyer struct{ UserID, Name, Email string }

// GoogleBuyerVerifier memverifikasi kredensial Google lalu mencari/membuat user. Tidak membuat sesi.
// Kredensial tidak valid atau email belum terverifikasi → AppError 401.
type GoogleBuyerVerifier interface {
	ResolveGoogleBuyer(ctx context.Context, credential, ip, userAgent string) (TenantBuyer, error)
}

// TenantBuyerWorkspaces mencari atau membuat workspace milik pembeli untuk SATU tenant.
type TenantBuyerWorkspaces interface {
	// FindForTenant: workspace milik user yang sudah tertaut ke company tenant ini, atau yang
	// dibuat oleh checkout tenant ini tetapi rantainya belum selesai.
	FindForTenant(ctx context.Context, tenantOrganizationID, userID string) (workspaceID string, found bool, err error)
	// CreateForTenant membuat workspace baru (pembeli = owner) yang ditandai berasal dari tenant ini.
	CreateForTenant(ctx context.Context, tenantOrganizationID string, buyer TenantBuyer, ip, userAgent string) (string, error)
}

// TenantProductGate: gerbang "perlu diperbarui" yang tidak bergantung ctx (catalog ProductService).
type TenantProductGate interface {
	ProductsNeedingUpdate(ctx context.Context, products []catalogdomain.Product) (map[string]bool, error)
}

// FeatureChecker: nil berarti org punya fitur.
type FeatureChecker interface {
	RequireFeature(ctx context.Context, organizationID, featureKey string) error
}

// WorkspaceSystemLinker: companyService.LinkWorkspaceBySystem.
type WorkspaceSystemLinker interface {
	LinkWorkspaceBySystem(ctx context.Context, scope coretenant.Scope, companyID, workspaceOrgID string) error
}

type TenantCheckoutDeps struct {
	Buyers       GoogleBuyerVerifier
	Workspaces   TenantBuyerWorkspaces
	Products     CatalogProducts
	ProductGate  TenantProductGate
	Entitlements FeatureChecker
	Platforms    PlatformScopeProvider
	Chain        SelfServeService
	Companies    SelfServeCompanies
	Links        WorkspaceSystemLinker
	Locker       Locker
}

// Fitur tenant yang wajib ada agar checkout publik boleh berjalan (sama dengan gerbang listing Task 8).
var tenantCheckoutRequiredFeatures = []string{"receivable.enabled", "receivable.online_payment"}

func NewTenantCheckoutService(deps TenantCheckoutDeps) TenantCheckoutService {
	return &tenantCheckoutService{deps: deps}
}

type tenantCheckoutService struct{ deps TenantCheckoutDeps }

func errTenantCheckoutNotFound() error {
	return coreerrors.New("TENANT_CHECKOUT_NOT_FOUND", "checkout tenant tidak ditemukan untuk host ini", http.StatusNotFound)
}

func errTenantProductNotFound() error {
	return coreerrors.New("TENANT_PRODUCT_NOT_FOUND", "Paket tidak ditemukan.", http.StatusNotFound)
}

func errTenantProductUnavailable() error {
	return coreerrors.New("TENANT_PRODUCT_UNAVAILABLE", "Paket ini tidak tersedia untuk dibeli langsung.", http.StatusUnprocessableEntity)
}

// Urutan sengaja: gerbang tanpa efek samping (host, fitur tenant, produk) dijalankan SEBELUM
// verifikasi Google yang bisa membuat user, supaya permintaan yang pasti ditolak tidak
// meninggalkan user/workspace yatim.
func (s *tenantCheckoutService) Checkout(ctx context.Context, tenant coretenant.Scope, in TenantCheckoutInput) (SelfServeCheckoutResult, error) {
	if err := s.rejectPlatform(ctx, tenant); err != nil {
		return SelfServeCheckoutResult{}, err
	}
	if err := s.requireTenantFeatures(ctx, tenant.OrganizationID()); err != nil {
		return SelfServeCheckoutResult{}, err
	}
	productID := strings.TrimSpace(in.ProductID)
	if err := s.requireProduct(ctx, tenant, productID); err != nil {
		return SelfServeCheckoutResult{}, err
	}

	buyer, err := s.deps.Buyers.ResolveGoogleBuyer(ctx, in.GoogleCredential, in.IP, in.UserAgent)
	if err != nil {
		return SelfServeCheckoutResult{}, err
	}

	// Satu lock per tenant+pembeli mencakup cari-atau-buat workspace, rantai CRM (yang membuat
	// company tertaut), dan tautan sistem. Rantai sendiri mengunci "self_serve:<workspace>".
	unlock, ok, err := s.deps.Locker.TryLock(ctx, "tenant_checkout:"+tenant.OrganizationID()+":"+buyer.UserID)
	if err != nil {
		return SelfServeCheckoutResult{}, err
	}
	if !ok {
		return SelfServeCheckoutResult{}, ErrSelfServeInProgress
	}
	defer unlock()

	workspaceID, err := s.ensureWorkspace(ctx, tenant, buyer, in)
	if err != nil {
		return SelfServeCheckoutResult{}, err
	}

	result, err := s.deps.Chain.Checkout(ctx, tenant, SelfServeCheckoutRequest{
		TenantOrganizationID: workspaceID, BuyerUserID: buyer.UserID, ProductID: productID,
	})
	if err != nil {
		return SelfServeCheckoutResult{}, err
	}
	if err := s.link(ctx, tenant, workspaceID); err != nil {
		return SelfServeCheckoutResult{}, err
	}
	return result, nil
}

func (s *tenantCheckoutService) rejectPlatform(ctx context.Context, tenant coretenant.Scope) error {
	platform, err := s.deps.Platforms.PlatformScope(ctx)
	if err != nil {
		return err
	}
	if platform.OrganizationID() == tenant.OrganizationID() {
		return errTenantCheckoutNotFound() // platform punya checkout mandirinya sendiri
	}
	return nil
}

func (s *tenantCheckoutService) requireTenantFeatures(ctx context.Context, tenantOrgID string) error {
	for _, key := range tenantCheckoutRequiredFeatures {
		if err := s.deps.Entitlements.RequireFeature(ctx, tenantOrgID, key); err != nil {
			return coreerrors.New("TENANT_CHECKOUT_DISABLED", "Pembelian online belum tersedia di sini.", http.StatusUnprocessableEntity)
		}
	}
	return nil
}

// requireProduct: produk milik tenant (query berfilter organization_id tenant), publik, aktif,
// bisa dibeli mandiri, dan tidak perlu diperbarui menurut katalog saat ini.
func (s *tenantCheckoutService) requireProduct(ctx context.Context, tenant coretenant.Scope, productID string) error {
	var id pgtype.UUID
	if id.Scan(productID) != nil || !id.Valid {
		return errTenantProductNotFound()
	}
	products, err := s.deps.Products.FindByIDs(ctx, tenant, []string{productID})
	if err != nil {
		return err
	}
	p, found := products[productID]
	if !found || p.DeletedAt != nil {
		return errTenantProductNotFound()
	}
	if err := validateSelfServeProduct(&p); err != nil {
		return errTenantProductUnavailable()
	}
	stale, err := s.deps.ProductGate.ProductsNeedingUpdate(ctx, []catalogdomain.Product{p})
	if err != nil {
		return err
	}
	if stale[p.ID] {
		return errTenantProductUnavailable()
	}
	return nil
}

func (s *tenantCheckoutService) ensureWorkspace(ctx context.Context, tenant coretenant.Scope, buyer TenantBuyer, in TenantCheckoutInput) (string, error) {
	id, found, err := s.deps.Workspaces.FindForTenant(ctx, tenant.OrganizationID(), buyer.UserID)
	if err != nil {
		return "", fmt.Errorf("find buyer workspace: %w", err)
	}
	if found {
		return id, nil
	}
	id, err = s.deps.Workspaces.CreateForTenant(ctx, tenant.OrganizationID(), buyer, in.IP, in.UserAgent)
	if err != nil {
		return "", fmt.Errorf("create buyer workspace: %w", err)
	}
	return id, nil
}

// link menegaskan tautan company tenant ↔ workspace pembeli lewat jalur sistem. Rantai membuat
// company tertaut secara atomik (ConvertCompanyInput.TenantOrganizationID), sehingga panggilan ini
// saat ini no-op idempoten (company dicari lewat tautan itu sendiri). Tetap dipanggil agar tautan
// selalu melewati satu jalur resmi; bila rantai kelak berhenti menautkan sendiri, langkah ini yang
// menautkan. Company yang belum ada setelah rantai sukses = galat langkah company (502).
func (s *tenantCheckoutService) link(ctx context.Context, tenant coretenant.Scope, workspaceID string) error {
	company, err := s.deps.Companies.FindByTenantOrganization(ctx, tenant, workspaceID)
	if err != nil {
		slog.ErrorContext(ctx, "tenant checkout: company for workspace not found after chain",
			"organization_id", tenant.OrganizationID(), "workspace_id", workspaceID, "error", err)
		return &SelfServeStepError{Step: SelfServeStepCompany, Err: err}
	}
	return s.deps.Links.LinkWorkspaceBySystem(ctx, tenant, company.ID, workspaceID)
}
