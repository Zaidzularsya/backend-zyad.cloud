package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/crm/domain"
)

const (
	tcTenantA  = "11111111-1111-4111-8111-111111111111"
	tcTenantB  = "22222222-2222-4222-8222-222222222222"
	tcPlatform = "99999999-9999-4999-8999-999999999999"
	tcProdA1   = "aaaaaaaa-0000-4000-8000-000000000001"
	tcProdA2   = "aaaaaaaa-0000-4000-8000-000000000002"
	tcProdB1   = "bbbbbbbb-0000-4000-8000-000000000001"
)

// tcWorld adalah fake di batas service untuk TenantCheckoutService. Rantai CRM (SelfServeService)
// disimulasikan: satu company per workspace (tertaut atomik), satu deal open per company.
type tcWorld struct {
	mu sync.Mutex

	buyers       map[string]TenantBuyer // credential -> buyer
	buyerErr     error
	users        map[string]bool // user yang pernah di-resolve
	products     map[string]map[string]catalogdomain.Product
	needsUpdate  map[string]bool
	missingFeats map[string]map[string]bool // org -> feature yang tidak dimiliki

	workspaces      map[string]string // tenant|user -> workspace
	workspaceCreate int
	companies       map[string]map[string]domain.Company // tenant -> workspace -> company
	deals           map[string]string                    // company -> deal
	checkouts       []SelfServeCheckoutRequest
	checkoutScopes  []string
	links           []string
	held            map[string]bool
	lockKeys        []string

	// chainGate, bila diisi, menahan rantai sampai ditutup (simulasi klik ganda bersamaan).
	chainStarted chan struct{}
	chainGate    chan struct{}
}

func newTCWorld() *tcWorld {
	pa1 := newProduct(tcProdA1, "WIFI-10")
	pa2 := newProduct(tcProdA2, "WIFI-20")
	pb1 := newProduct(tcProdB1, "OTHER-10")
	return &tcWorld{
		buyers: map[string]TenantBuyer{
			"cred-rina": {UserID: "user-rina", Name: "Rina", Email: "rina@example.com"},
			"cred-budi": {UserID: "user-budi", Name: "Budi", Email: "budi@example.com"},
		},
		users: map[string]bool{},
		products: map[string]map[string]catalogdomain.Product{
			tcTenantA: {tcProdA1: pa1, tcProdA2: pa2},
			tcTenantB: {tcProdB1: pb1},
		},
		needsUpdate:  map[string]bool{},
		missingFeats: map[string]map[string]bool{},
		workspaces:   map[string]string{},
		companies:    map[string]map[string]domain.Company{},
		deals:        map[string]string{},
		held:         map[string]bool{},
	}
}

// ---- GoogleBuyerVerifier
func (w *tcWorld) ResolveGoogleBuyer(_ context.Context, credential, _, _ string) (TenantBuyer, error) {
	if w.buyerErr != nil {
		return TenantBuyer{}, w.buyerErr
	}
	b, ok := w.buyers[credential]
	if !ok {
		return TenantBuyer{}, coreerrors.New("AUTH_GOOGLE_TOKEN_INVALID", "invalid", http.StatusUnauthorized)
	}
	w.mu.Lock()
	w.users[b.UserID] = true
	w.mu.Unlock()
	return b, nil
}

// ---- TenantBuyerWorkspaces
func (w *tcWorld) FindForTenant(_ context.Context, tenantOrgID, userID string) (string, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	id, ok := w.workspaces[tenantOrgID+"|"+userID]
	return id, ok, nil
}
func (w *tcWorld) CreateForTenant(_ context.Context, tenantOrgID string, buyer TenantBuyer, _, _ string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.workspaceCreate++
	id := fmt.Sprintf("ws-%d", w.workspaceCreate)
	w.workspaces[tenantOrgID+"|"+buyer.UserID] = id
	return id, nil
}

// ---- products / gates
func (w *tcWorld) FindByIDs(_ context.Context, scope coretenant.Scope, ids []string) (map[string]catalogdomain.Product, error) {
	out := map[string]catalogdomain.Product{}
	for _, id := range ids {
		if p, ok := w.products[scope.OrganizationID()][id]; ok {
			out[id] = p
		}
	}
	return out, nil
}
func (w *tcWorld) ProductsNeedingUpdate(_ context.Context, products []catalogdomain.Product) (map[string]bool, error) {
	out := map[string]bool{}
	for _, p := range products {
		if w.needsUpdate[p.ID] {
			out[p.ID] = true
		}
	}
	return out, nil
}
func (w *tcWorld) RequireFeature(_ context.Context, organizationID, featureKey string) error {
	if w.missingFeats[organizationID][featureKey] {
		return errors.New("feature not enabled")
	}
	return nil
}
func (w *tcWorld) PlatformScope(context.Context) (coretenant.Scope, error) {
	return tcScope(tcPlatform), nil
}

// ---- rantai CRM (SelfServeService) + company lookup + link
func (w *tcWorld) Checkout(_ context.Context, scope coretenant.Scope, req SelfServeCheckoutRequest) (SelfServeCheckoutResult, error) {
	if w.chainStarted != nil {
		close(w.chainStarted)
		<-w.chainGate
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	tenant := scope.OrganizationID()
	w.checkouts = append(w.checkouts, req)
	w.checkoutScopes = append(w.checkoutScopes, tenant)
	if w.companies[tenant] == nil {
		w.companies[tenant] = map[string]domain.Company{}
	}
	company, ok := w.companies[tenant][req.TenantOrganizationID]
	if !ok {
		ws := req.TenantOrganizationID
		company = domain.Company{ID: fmt.Sprintf("co-%d", len(w.companies[tenant])+1), TenantOrganizationID: &ws}
		w.companies[tenant][req.TenantOrganizationID] = company
	}
	deal, ok := w.deals[company.ID]
	if !ok {
		deal = "deal-" + company.ID
		w.deals[company.ID] = deal
	}
	return SelfServeCheckoutResult{InvoiceURL: "https://pay.example/" + deal + "/" + req.ProductID, DealID: deal}, nil
}
func (w *tcWorld) FindByTenantOrganization(_ context.Context, scope coretenant.Scope, workspaceID string) (domain.Company, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.companies[scope.OrganizationID()][workspaceID]
	if !ok {
		return domain.Company{}, pgx.ErrNoRows
	}
	return c, nil
}
func (w *tcWorld) LinkWorkspaceBySystem(_ context.Context, scope coretenant.Scope, companyID, workspaceOrgID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.links = append(w.links, scope.OrganizationID()+"|"+companyID+"|"+workspaceOrgID)
	return nil
}

// FindWorkspaceLinkAnyOrg: company mana pun (semua org) yang tertaut ke workspace.
func (w *tcWorld) FindWorkspaceLinkAnyOrg(_ context.Context, workspaceOrgID string) (string, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, byWS := range w.companies {
		if c, ok := byWS[workspaceOrgID]; ok {
			return c.ID, true, nil
		}
	}
	return "", false, nil
}

// ---- Locker: lock in-memory yang benar-benar eksklusif.
func (w *tcWorld) TryLock(_ context.Context, key string) (func(), bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lockKeys = append(w.lockKeys, key)
	if w.held[key] {
		return nil, false, nil
	}
	w.held[key] = true
	return func() {
		w.mu.Lock()
		delete(w.held, key)
		w.mu.Unlock()
	}, true, nil
}

func tcScope(orgID string) coretenant.Scope {
	typ := coretenant.OrganizationTypeCustomer
	if orgID == tcPlatform {
		typ = coretenant.OrganizationTypePlatform
	}
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: orgID, OrganizationSlug: "org-" + orgID[:4], OrganizationType: typ,
		OrganizationStatus: coretenant.OrganizationStatusActive, ResolutionSource: coretenant.ResolutionSourceInternal,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		panic(err)
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		panic(err)
	}
	return scope
}

func newTC(w *tcWorld) TenantCheckoutService {
	return NewTenantCheckoutService(TenantCheckoutDeps{
		Buyers: w, Workspaces: w, Products: w, ProductGate: w, Entitlements: w, Platforms: w,
		Chain: w, Companies: w, Links: w, WorkspaceLinks: w, Locker: w,
	})
}

func tcAppErr(t *testing.T, err error) (string, int) {
	t.Helper()
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %v", err)
	}
	return appErr.Code, appErr.Status
}

func TestTenantCheckout_NewBuyerCreatesWorkspaceChainAndLink(t *testing.T) {
	w := newTCWorld()
	res, err := newTC(w).Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1, IP: "203.0.113.1"})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if res.InvoiceURL == "" {
		t.Fatal("expected invoice url")
	}
	if !w.users["user-rina"] || w.workspaceCreate != 1 {
		t.Fatalf("users=%v workspaceCreate=%d", w.users, w.workspaceCreate)
	}
	if len(w.checkouts) != 1 {
		t.Fatalf("checkouts=%v", w.checkouts)
	}
	got := w.checkouts[0]
	if got.TenantOrganizationID != "ws-1" || got.BuyerUserID != "user-rina" || got.ProductID != tcProdA1 {
		t.Fatalf("chain request = %+v", got)
	}
	if w.checkoutScopes[0] != tcTenantA {
		t.Fatalf("chain must run in the tenant scope, ran in %s", w.checkoutScopes[0])
	}
	if len(w.links) != 1 || w.links[0] != tcTenantA+"|co-1|ws-1" {
		t.Fatalf("links=%v", w.links)
	}
	if len(w.held) != 0 {
		t.Fatalf("locks must be released, held=%v", w.held)
	}
}

func TestTenantCheckout_RepeatIsIdempotent(t *testing.T) {
	w := newTCWorld()
	svc := newTC(w)
	in := TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1}
	first, err := svc.Checkout(context.Background(), tcScope(tcTenantA), in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Checkout(context.Background(), tcScope(tcTenantA), in)
	if err != nil {
		t.Fatal(err)
	}
	if w.workspaceCreate != 1 || len(w.companies[tcTenantA]) != 1 || len(w.deals) != 1 {
		t.Fatalf("workspaces=%d companies=%d deals=%d", w.workspaceCreate, len(w.companies[tcTenantA]), len(w.deals))
	}
	if first.InvoiceURL != second.InvoiceURL {
		t.Fatalf("invoice url changed: %s vs %s", first.InvoiceURL, second.InvoiceURL)
	}
}

func TestTenantCheckout_SecondProductReusesWorkspace(t *testing.T) {
	w := newTCWorld()
	svc := newTC(w)
	if _, err := svc.Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA2}); err != nil {
		t.Fatal(err)
	}
	if w.workspaceCreate != 1 || w.checkouts[1].TenantOrganizationID != "ws-1" {
		t.Fatalf("workspaceCreate=%d checkouts=%+v", w.workspaceCreate, w.checkouts)
	}
}

func TestTenantCheckout_SameBuyerOtherTenantGetsOwnWorkspace(t *testing.T) {
	w := newTCWorld()
	svc := newTC(w)
	if _, err := svc.Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Checkout(context.Background(), tcScope(tcTenantB), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdB1}); err != nil {
		t.Fatal(err)
	}
	if w.workspaceCreate != 2 || w.checkouts[1].TenantOrganizationID != "ws-2" {
		t.Fatalf("another tenant must not reuse the first workspace: create=%d checkouts=%+v", w.workspaceCreate, w.checkouts)
	}
}

func TestTenantCheckout_UnverifiedEmailRejected(t *testing.T) {
	w := newTCWorld()
	w.buyerErr = coreerrors.New("AUTH_GOOGLE_EMAIL_UNVERIFIED", "google email is not verified", http.StatusUnauthorized)
	_, err := newTC(w).Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1})
	if code, status := tcAppErr(t, err); code != "AUTH_GOOGLE_EMAIL_UNVERIFIED" || status != http.StatusUnauthorized {
		t.Fatalf("code=%s status=%d", code, status)
	}
	if w.workspaceCreate != 0 || len(w.checkouts) != 0 {
		t.Fatal("nothing may be created for an unverified buyer")
	}
}

func TestTenantCheckout_ProductOfOtherTenantNotFound(t *testing.T) {
	w := newTCWorld()
	_, err := newTC(w).Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdB1})
	if code, status := tcAppErr(t, err); code != "TENANT_PRODUCT_NOT_FOUND" || status != http.StatusNotFound {
		t.Fatalf("code=%s status=%d", code, status)
	}
	if len(w.users) != 0 || w.workspaceCreate != 0 {
		t.Fatal("no user or workspace may be created for an unknown product")
	}
}

func TestTenantCheckout_MalformedProductIDNotFound(t *testing.T) {
	w := newTCWorld()
	_, err := newTC(w).Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: "not-a-uuid"})
	if code, status := tcAppErr(t, err); code != "TENANT_PRODUCT_NOT_FOUND" || status != http.StatusNotFound {
		t.Fatalf("code=%s status=%d", code, status)
	}
}

func TestTenantCheckout_ProductNeedingUpdateRejected(t *testing.T) {
	w := newTCWorld()
	w.needsUpdate[tcProdA1] = true
	_, err := newTC(w).Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1})
	if code, status := tcAppErr(t, err); code != "TENANT_PRODUCT_UNAVAILABLE" || status != http.StatusUnprocessableEntity {
		t.Fatalf("code=%s status=%d", code, status)
	}
}

func TestTenantCheckout_ProductNotCheckoutCapableRejected(t *testing.T) {
	for name, mutate := range map[string]func(*catalogdomain.Product){
		"not public": func(p *catalogdomain.Product) { p.IsPublic = false },
		"inactive":   func(p *catalogdomain.Product) { p.IsActive = false },
	} {
		t.Run(name, func(t *testing.T) {
			w := newTCWorld()
			p := w.products[tcTenantA][tcProdA1]
			mutate(&p)
			w.products[tcTenantA][tcProdA1] = p
			_, err := newTC(w).Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1})
			if code, status := tcAppErr(t, err); code != "TENANT_PRODUCT_UNAVAILABLE" || status != http.StatusUnprocessableEntity {
				t.Fatalf("code=%s status=%d", code, status)
			}
		})
	}
}

func TestTenantCheckout_TenantWithoutOnlinePaymentRejected(t *testing.T) {
	for _, key := range []string{"receivable.enabled", "receivable.online_payment"} {
		t.Run(key, func(t *testing.T) {
			w := newTCWorld()
			w.missingFeats[tcTenantA] = map[string]bool{key: true}
			_, err := newTC(w).Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1})
			if code, status := tcAppErr(t, err); code != "TENANT_CHECKOUT_DISABLED" || status != http.StatusUnprocessableEntity {
				t.Fatalf("code=%s status=%d", code, status)
			}
			if len(w.users) != 0 {
				t.Fatal("no user may be created when the tenant cannot take payments")
			}
		})
	}
}

func TestTenantCheckout_PlatformScopeNotFound(t *testing.T) {
	w := newTCWorld()
	_, err := newTC(w).Checkout(context.Background(), tcScope(tcPlatform), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1})
	if code, status := tcAppErr(t, err); code != "TENANT_CHECKOUT_NOT_FOUND" || status != http.StatusNotFound {
		t.Fatalf("code=%s status=%d", code, status)
	}
}

func TestTenantCheckout_LockCoversTenantAndBuyer(t *testing.T) {
	w := newTCWorld()
	if _, err := newTC(w).Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1}); err != nil {
		t.Fatal(err)
	}
	want := "tenant_checkout:" + tcTenantA + ":user-rina"
	if len(w.lockKeys) == 0 || w.lockKeys[0] != want {
		t.Fatalf("lock keys = %v, want first %s", w.lockKeys, want)
	}
}

func TestTenantCheckout_ConcurrentCallsYieldOneWorkspace(t *testing.T) {
	w := newTCWorld()
	w.chainStarted = make(chan struct{})
	w.chainGate = make(chan struct{})
	svc := newTC(w)
	in := TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1}

	done := make(chan error, 1)
	go func() {
		_, err := svc.Checkout(context.Background(), tcScope(tcTenantA), in)
		done <- err
	}()
	<-w.chainStarted // panggilan pertama memegang lock dan sedang di tengah rantai

	_, err := svc.Checkout(context.Background(), tcScope(tcTenantA), in)
	if !errors.Is(err, ErrSelfServeInProgress) {
		t.Fatalf("second concurrent call: want ErrSelfServeInProgress, got %v", err)
	}
	close(w.chainGate)
	if err := <-done; err != nil {
		t.Fatalf("first call: %v", err)
	}
	if w.workspaceCreate != 1 || len(w.companies[tcTenantA]) != 1 || len(w.deals) != 1 {
		t.Fatalf("workspaces=%d companies=%d deals=%d", w.workspaceCreate, len(w.companies[tcTenantA]), len(w.deals))
	}
}

func TestTenantCheckout_ChainErrorPropagates(t *testing.T) {
	w := newTCWorld()
	svc := NewTenantCheckoutService(TenantCheckoutDeps{
		Buyers: w, Workspaces: w, Products: w, ProductGate: w, Entitlements: w, Platforms: w,
		Chain: failingChain{err: ErrSelfServeAlreadySubscribed}, Companies: w, Links: w, WorkspaceLinks: w, Locker: w,
	})
	_, err := svc.Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1})
	if !errors.Is(err, ErrSelfServeAlreadySubscribed) {
		t.Fatalf("got %v", err)
	}
	if len(w.links) != 0 {
		t.Fatal("no link after a failed chain")
	}
}

type failingChain struct{ err error }

func (f failingChain) Checkout(context.Context, coretenant.Scope, SelfServeCheckoutRequest) (SelfServeCheckoutResult, error) {
	return SelfServeCheckoutResult{}, f.err
}

// Spec §4.2: produk tenant tanpa entitlement (mis. paket WiFi) sah dibeli.
func TestTenantCheckout_FeaturelessProductEndToEnd(t *testing.T) {
	w := newTCWorld()
	p := w.products[tcTenantA][tcProdA1]
	p.Features = nil
	w.products[tcTenantA][tcProdA1] = p
	res, err := newTC(w).Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if res.InvoiceURL == "" || w.workspaceCreate != 1 || len(w.checkouts) != 1 || len(w.links) != 1 {
		t.Fatalf("res=%+v workspaces=%d checkouts=%d links=%d", res, w.workspaceCreate, len(w.checkouts), len(w.links))
	}
}

func TestTenantCheckout_PostpaidProductRejected(t *testing.T) {
	w := newTCWorld()
	p := w.products[tcTenantA][tcProdA1]
	p.Pricing.PaymentTiming = "postpaid"
	w.products[tcTenantA][tcProdA1] = p
	_, err := newTC(w).Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1})
	if code, status := tcAppErr(t, err); code != "TENANT_PRODUCT_UNAVAILABLE" || status != http.StatusUnprocessableEntity {
		t.Fatalf("code=%s status=%d", code, status)
	}
}

// Workspace pembeli yang sudah tertaut ke company org LAIN ditolak SEBELUM rantai berjalan
// (rantai menautkan di dalam Convert, sehingga guard di LinkWorkspaceBySystem tidak pernah
// sempat berjalan pada jalur checkout).
func TestTenantCheckout_WorkspaceLinkedInOtherOrgIsRejected(t *testing.T) {
	w := newTCWorld()
	w.workspaces[tcTenantA+"|user-rina"] = "ws-x"
	ws := "ws-x"
	w.companies[tcTenantB] = map[string]domain.Company{"ws-x": {ID: "co-b", TenantOrganizationID: &ws}}

	_, err := newTC(w).Checkout(context.Background(), tcScope(tcTenantA), TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1})
	if code, status := tcAppErr(t, err); code != "WORKSPACE_ALREADY_LINKED" || status != http.StatusConflict {
		t.Fatalf("got %s %d", code, status)
	}
	if len(w.checkouts) != 0 || len(w.links) != 0 {
		t.Fatalf("chain must not run: checkouts=%v links=%v", w.checkouts, w.links)
	}
	if len(w.held) != 0 {
		t.Fatalf("locks must be released, held=%v", w.held)
	}
}

// Tautan ke company tenant ini sendiri = pembeli lama; checkout dilanjutkan.
func TestTenantCheckout_WorkspaceLinkedInSameTenantProceeds(t *testing.T) {
	w := newTCWorld()
	svc := newTC(w)
	in := TenantCheckoutInput{GoogleCredential: "cred-rina", ProductID: tcProdA1}
	if _, err := svc.Checkout(context.Background(), tcScope(tcTenantA), in); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Checkout(context.Background(), tcScope(tcTenantA), in); err != nil {
		t.Fatalf("same-tenant link must be reused: %v", err)
	}
	if len(w.checkouts) != 2 || w.workspaceCreate != 1 {
		t.Fatalf("checkouts=%d workspaces=%d", len(w.checkouts), w.workspaceCreate)
	}
}
