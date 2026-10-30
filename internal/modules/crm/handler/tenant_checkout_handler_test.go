package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/service"
)

const (
	tcHostTenantA  = "11111111-1111-4111-8111-111111111111"
	tcHostTenantB  = "22222222-2222-4222-8222-222222222222"
	tcHostPlatform = "99999999-9999-4999-8999-999999999999"
)

type tcHostResolver struct{ t *testing.T }

func (h tcHostResolver) ResolvePublicHost(_ context.Context, host string) (coretenant.Context, bool, error) {
	var id string
	typ := coretenant.OrganizationTypeCustomer
	switch host {
	case "a.example.com":
		id = tcHostTenantA
	case "b.example.com":
		id = tcHostTenantB
	case "platform.example.com":
		id, typ = tcHostPlatform, coretenant.OrganizationTypePlatform
	default:
		return coretenant.Context{}, false, nil
	}
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: id, OrganizationSlug: "org-" + id[:4], OrganizationType: typ, OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", MembershipStatus: "active", MembershipVersion: 1,
		ResolutionSource: coretenant.ResolutionSourceCustomDomain, DataPlacement: coretenant.DataPlacementShared, RequestHost: host,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return tc, true, nil
}

type fakeTenantCheckoutSvc struct {
	mu     sync.Mutex
	scopes []string
	inputs []service.TenantCheckoutInput
	err    error
}

func (f *fakeTenantCheckoutSvc) Checkout(_ context.Context, tenant coretenant.Scope, in service.TenantCheckoutInput) (service.SelfServeCheckoutResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scopes = append(f.scopes, tenant.OrganizationID())
	f.inputs = append(f.inputs, in)
	if f.err != nil {
		return service.SelfServeCheckoutResult{}, f.err
	}
	return service.SelfServeCheckoutResult{InvoiceURL: "https://pay.example/inv", DealID: "deal-secret"}, nil
}

type tcCounter struct {
	mu     sync.Mutex
	counts map[string]int64
}

func (f *tcCounter) Incr(ctx context.Context, key string) *goredis.IntCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd := goredis.NewIntCmd(ctx)
	f.counts[key]++
	cmd.SetVal(f.counts[key])
	return cmd
}

func (f *tcCounter) Expire(ctx context.Context, _ string, _ time.Duration) *goredis.BoolCmd {
	return goredis.NewBoolCmd(ctx)
}

func tenantCheckoutRouter(t *testing.T, svc service.TenantCheckoutService, counter middleware.RateCounter) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mw, err := middleware.ResolvePublicOrganization(tcHostResolver{t}, middleware.PublicHostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	NewTenantCheckoutHandler(svc, counter).RegisterRoutes(r.Group("", mw, middleware.RequireActiveTenant()))
	return r
}

func postCheckout(r http.Handler, host, ip, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/public/tenant-checkout", strings.NewReader(body))
	req.Host = host
	req.RemoteAddr = ip + ":5555"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "test-agent")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const tcBody = `{"google_credential":"cred","product_id":"aaaaaaaa-0000-4000-8000-000000000001"}`

func TestTenantCheckoutHandler_Success(t *testing.T) {
	svc := &fakeTenantCheckoutSvc{}
	w := postCheckout(tenantCheckoutRouter(t, svc, nil), "a.example.com", "203.0.113.5", tcBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data["invoice_url"] != "https://pay.example/inv" {
		t.Fatalf("body=%s", w.Body.String())
	}
	if _, leaked := out.Data["deal_id"]; leaked {
		t.Fatal("tenant CRM deal id must not be exposed to the public buyer")
	}
	in := svc.inputs[0]
	if in.GoogleCredential != "cred" || in.ProductID != "aaaaaaaa-0000-4000-8000-000000000001" || in.IP != "203.0.113.5" || in.UserAgent != "test-agent" {
		t.Fatalf("input=%+v", in)
	}
	if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("Cache-Control=%q", got)
	}
}

func TestTenantCheckoutHandler_TenantFromHostIgnoresBodyOrganization(t *testing.T) {
	svc := &fakeTenantCheckoutSvc{}
	body := `{"google_credential":"cred","product_id":"aaaaaaaa-0000-4000-8000-000000000001","organization_id":"` + tcHostTenantB + `"}`
	r := tenantCheckoutRouter(t, svc, nil)
	req := httptest.NewRequest(http.MethodPost, "/public/tenant-checkout?organization_id="+tcHostTenantB, strings.NewReader(body))
	req.Host = "a.example.com"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if len(svc.scopes) != 1 || svc.scopes[0] != tcHostTenantA {
		t.Fatalf("scopes=%v", svc.scopes)
	}
}

func TestTenantCheckoutHandler_PlatformHost404(t *testing.T) {
	svc := &fakeTenantCheckoutSvc{}
	w := postCheckout(tenantCheckoutRouter(t, svc, nil), "platform.example.com", "203.0.113.5", tcBody)
	if w.Code != http.StatusNotFound || len(svc.scopes) != 0 {
		t.Fatalf("code=%d calls=%v", w.Code, svc.scopes)
	}
}

func TestTenantCheckoutHandler_UnknownHost404(t *testing.T) {
	svc := &fakeTenantCheckoutSvc{}
	w := postCheckout(tenantCheckoutRouter(t, svc, nil), "nobody.example.com", "203.0.113.5", tcBody)
	if w.Code != http.StatusNotFound || len(svc.scopes) != 0 {
		t.Fatalf("code=%d calls=%v", w.Code, svc.scopes)
	}
}

func TestTenantCheckoutHandler_ValidatesBody(t *testing.T) {
	for _, body := range []string{`{}`, `{"google_credential":"cred"}`, `{"product_id":"x"}`, `not json`} {
		svc := &fakeTenantCheckoutSvc{}
		w := postCheckout(tenantCheckoutRouter(t, svc, nil), "a.example.com", "203.0.113.5", body)
		if w.Code != http.StatusUnprocessableEntity || len(svc.scopes) != 0 {
			t.Fatalf("body %s: code=%d", body, w.Code)
		}
	}
}

func TestTenantCheckoutHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{coreerrors.New("AUTH_GOOGLE_EMAIL_UNVERIFIED", "x", http.StatusUnauthorized), http.StatusUnauthorized},
		{coreerrors.New("TENANT_PRODUCT_NOT_FOUND", "x", http.StatusNotFound), http.StatusNotFound},
		{coreerrors.New("TENANT_PRODUCT_UNAVAILABLE", "x", http.StatusUnprocessableEntity), http.StatusUnprocessableEntity},
		{coreerrors.New("TENANT_CHECKOUT_DISABLED", "x", http.StatusUnprocessableEntity), http.StatusUnprocessableEntity},
		{service.ErrSelfServeInProgress, http.StatusConflict},
		{service.ErrSelfServeAlreadySubscribed, http.StatusConflict},
		{service.ErrSelfServeNotConfigured, http.StatusServiceUnavailable},
		{&service.SelfServeStepError{Step: "invoice", Err: context.DeadlineExceeded}, http.StatusBadGateway},
	}
	for _, tc := range cases {
		svc := &fakeTenantCheckoutSvc{err: tc.err}
		w := postCheckout(tenantCheckoutRouter(t, svc, nil), "a.example.com", "203.0.113.5", tcBody)
		if w.Code != tc.code {
			t.Fatalf("%v: code=%d want %d body=%s", tc.err, w.Code, tc.code, w.Body.String())
		}
	}
}

func TestTenantCheckoutHandler_RateLimitPerIP(t *testing.T) {
	svc := &fakeTenantCheckoutSvc{}
	r := tenantCheckoutRouter(t, svc, &tcCounter{counts: map[string]int64{}})
	for i := 0; i < tenantCheckoutIPLimit; i++ {
		if w := postCheckout(r, "a.example.com", "203.0.113.7", tcBody); w.Code != http.StatusCreated {
			t.Fatalf("request %d: code=%d", i+1, w.Code)
		}
	}
	if w := postCheckout(r, "a.example.com", "203.0.113.7", tcBody); w.Code != http.StatusTooManyRequests {
		t.Fatalf("over the IP limit: code=%d", w.Code)
	}
	// IP lain tetap boleh.
	if w := postCheckout(r, "a.example.com", "203.0.113.8", tcBody); w.Code != http.StatusCreated {
		t.Fatalf("other IP: code=%d", w.Code)
	}
}

func TestTenantCheckoutHandler_RateLimitPerTenant(t *testing.T) {
	svc := &fakeTenantCheckoutSvc{}
	r := tenantCheckoutRouter(t, svc, &tcCounter{counts: map[string]int64{}})
	for i := 0; i < tenantCheckoutTenantLimit; i++ {
		ip := "198.51." + itoa(i/250) + "." + itoa(i%250+1) // IP berbeda: hanya batas tenant yang bekerja
		if w := postCheckout(r, "a.example.com", ip, tcBody); w.Code != http.StatusCreated {
			t.Fatalf("request %d: code=%d", i+1, w.Code)
		}
	}
	if w := postCheckout(r, "a.example.com", "192.0.2.200", tcBody); w.Code != http.StatusTooManyRequests {
		t.Fatalf("over the tenant limit: code=%d", w.Code)
	}
	// Tenant lain punya kuota sendiri.
	if w := postCheckout(r, "b.example.com", "192.0.2.200", tcBody); w.Code != http.StatusCreated {
		t.Fatalf("other tenant: code=%d", w.Code)
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}
