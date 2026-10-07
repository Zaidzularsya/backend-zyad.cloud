package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/service"
)

// ssGrant meniru checker organisasi: izin yang tidak diberikan → 403 AppError (seperti implementasi nyata).
type ssGrant map[string]bool

func (g ssGrant) CanOrganization(_ context.Context, _, _ string, perms []string) error {
	for _, p := range perms {
		if !g[p] {
			return coreerrors.New("FORBIDDEN", "permission denied", http.StatusForbidden)
		}
	}
	return nil
}

type ssSvcFake struct {
	req    service.SelfServeCheckoutRequest
	scope  coretenant.Scope
	result service.SelfServeCheckoutResult
	err    error
	calls  int
}

func (f *ssSvcFake) Checkout(_ context.Context, scope coretenant.Scope, req service.SelfServeCheckoutRequest) (service.SelfServeCheckoutResult, error) {
	f.calls++
	f.req, f.scope = req, scope
	return f.result, f.err
}

type ssPlatformFake struct{ scope coretenant.Scope }

func (f ssPlatformFake) PlatformScope(context.Context) (coretenant.Scope, error) { return f.scope, nil }

func platformTenantContext(t *testing.T) coretenant.Context {
	t.Helper()
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "99999999-9999-9999-9999-999999999999", OrganizationSlug: "platform",
		OrganizationType: coretenant.OrganizationTypePlatform, OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", MembershipStatus: "active", MembershipVersion: 1,
		ResolutionSource: coretenant.ResolutionSourceSession, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tc
}

func newSelfServeRouter(t *testing.T, svc service.SelfServeService, perms ...string) (*gin.Engine, *ssSvcFake) {
	t.Helper()
	r := newTestRouter(t) // tenant customer 1111...
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "buyer-1"); c.Next() })
	g := ssGrant{}
	for _, p := range perms {
		g[p] = true
	}
	fake, _ := svc.(*ssSvcFake)
	NewSelfServeHandler(svc, ssPlatformFake{scope: coretenant.Scope{}}, g).RegisterRoutes(&r.RouterGroup)
	return r, fake
}

func errCode(t *testing.T, body string) (string, map[string]any) {
	t.Helper()
	var env struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return env.Code, env.Data
}

func TestSelfServeCheckoutSuccess(t *testing.T) {
	svc := &ssSvcFake{result: service.SelfServeCheckoutResult{InvoiceURL: "https://pay.test/x", DealID: "deal-1"}}
	r, _ := newSelfServeRouter(t, svc, "organization.billing.manage")
	w := performJSON(r, http.MethodPost, "/app/self-serve/checkout", `{"product_id":"p1"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", w.Code, w.Body)
	}
	var env struct {
		Data struct {
			InvoiceURL string `json:"invoice_url"`
			DealID     string `json:"deal_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if env.Data.InvoiceURL != "https://pay.test/x" || env.Data.DealID != "deal-1" {
		t.Fatalf("data = %+v", env.Data)
	}
	if svc.req.TenantOrganizationID != "11111111-1111-1111-1111-111111111111" || svc.req.BuyerUserID != "buyer-1" || svc.req.ProductID != "p1" {
		t.Fatalf("req = %+v", svc.req)
	}
}

func TestSelfServeCheckoutRequiresBillingPermission(t *testing.T) {
	svc := &ssSvcFake{}
	r, _ := newSelfServeRouter(t, svc) // tanpa izin
	w := performJSON(r, http.MethodPost, "/app/self-serve/checkout", `{"product_id":"p1"}`)
	if w.Code != http.StatusForbidden || svc.calls != 0 {
		t.Fatalf("status = %d calls=%d", w.Code, svc.calls)
	}
}

func TestSelfServeCheckoutRejectsPlatformOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &ssSvcFake{}
	r := gin.New()
	tc := platformTenantContext(t)
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
		permissionmiddleware.SetUserID(c, "admin")
		c.Next()
	})
	NewSelfServeHandler(svc, ssPlatformFake{}, ssGrant{"organization.billing.manage": true}).RegisterRoutes(&r.RouterGroup)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/app/self-serve/checkout", strings.NewReader(`{"product_id":"p1"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || svc.calls != 0 {
		t.Fatalf("status = %d calls=%d body=%s", w.Code, svc.calls, w.Body)
	}
}

func TestSelfServeCheckoutValidation(t *testing.T) {
	for _, body := range []string{`{}`, `{"product_id":""}`, `not json`} {
		svc := &ssSvcFake{}
		r, _ := newSelfServeRouter(t, svc, "organization.billing.manage")
		w := performJSON(r, http.MethodPost, "/app/self-serve/checkout", body)
		if w.Code != http.StatusUnprocessableEntity || svc.calls != 0 {
			t.Errorf("%q: status = %d calls=%d", body, w.Code, svc.calls)
		}
	}
}

func TestSelfServeCheckoutErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
		step   string
	}{
		{service.ErrSelfServeProductUnavailable, 422, "SELF_SERVE_PRODUCT_UNAVAILABLE", ""},
		{service.ErrSelfServeAlreadySubscribed, 409, "SELF_SERVE_ALREADY_SUBSCRIBED", ""},
		{service.ErrSelfServeInProgress, 409, "SELF_SERVE_IN_PROGRESS", ""},
		{service.ErrSelfServeNotConfigured, 503, "SELF_SERVE_NOT_CONFIGURED", ""},
		{&service.SelfServeStepError{Step: "quotation", Err: errors.New("secret db detail")}, 502, "SELF_SERVE_STEP_FAILED", "quotation"},
	}
	for _, c := range cases {
		svc := &ssSvcFake{err: c.err}
		r, _ := newSelfServeRouter(t, svc, "organization.billing.manage")
		w := performJSON(r, http.MethodPost, "/app/self-serve/checkout", `{"product_id":"p1"}`)
		code, data := errCode(t, w.Body.String())
		if w.Code != c.status || code != c.code {
			t.Errorf("%v: status=%d code=%s", c.err, w.Code, code)
		}
		if c.step != "" && data["step"] != c.step {
			t.Errorf("step = %v", data["step"])
		}
		if strings.Contains(w.Body.String(), "secret db detail") {
			t.Errorf("original error leaked to client: %s", w.Body)
		}
	}
}

func TestSelfServeCheckoutNotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "buyer-1"); c.Next() })
	NewSelfServeHandler(nil, ssPlatformFake{}, ssGrant{"organization.billing.manage": true}).RegisterRoutes(&r.RouterGroup)
	w := performJSON(r, http.MethodPost, "/app/self-serve/checkout", `{"product_id":"p1"}`)
	code, _ := errCode(t, w.Body.String())
	if w.Code != http.StatusServiceUnavailable || code != "SELF_SERVE_NOT_CONFIGURED" {
		t.Fatalf("status=%d code=%s", w.Code, code)
	}
}

type ssSubFake struct {
	view   service.SubscriptionView
	tenant string
	calls  int
}

func (f *ssSubFake) Subscription(_ context.Context, _ coretenant.Scope, tenantOrgID string, _ time.Time, _ int) (service.SubscriptionView, error) {
	f.calls++
	f.tenant = tenantOrgID
	return f.view, nil
}

func TestSelfServeSubscriptionUsesTenantFromContext(t *testing.T) {
	sub := &ssSubFake{view: service.SubscriptionView{Status: "overdue", OverdueDays: 3, SuspendInDays: ptr(5),
		Invoices: []service.SubscriptionInvoice{{Number: "INV-1", Status: "overdue", Total: "333000.00"}}}}
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "buyer-1"); c.Next() })
	NewSelfServeHandler(&ssSvcFake{}, ssPlatformFake{}, ssGrant{"organization.billing.read": true}).
		WithSubscription(sub, 7).RegisterRoutes(&r.RouterGroup)
	// parameter query tidak boleh menggeser tenant
	w := performJSON(r, http.MethodGet, "/app/self-serve/subscription?tenant_organization_id=other", "")
	if w.Code != http.StatusOK || sub.tenant != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("status=%d tenant=%q body=%s", w.Code, sub.tenant, w.Body)
	}
	var env struct {
		Data struct {
			Status        string `json:"status"`
			OverdueDays   int    `json:"overdue_days"`
			SuspendInDays *int   `json:"suspend_in_days"`
			Features      []any  `json:"features"`
			Invoices      []struct {
				Number string `json:"number"`
			} `json:"invoices"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if env.Data.Status != "overdue" || env.Data.OverdueDays != 3 || env.Data.SuspendInDays == nil || *env.Data.SuspendInDays != 5 ||
		env.Data.Features == nil || len(env.Data.Invoices) != 1 || env.Data.Invoices[0].Number != "INV-1" {
		t.Fatalf("data = %+v", env.Data)
	}
}

func TestSelfServeSubscriptionRequiresBillingRead(t *testing.T) {
	sub := &ssSubFake{}
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "buyer-1"); c.Next() })
	NewSelfServeHandler(&ssSvcFake{}, ssPlatformFake{}, ssGrant{}).WithSubscription(sub, 7).RegisterRoutes(&r.RouterGroup)
	if w := performJSON(r, http.MethodGet, "/app/self-serve/subscription", ""); w.Code != http.StatusForbidden || sub.calls != 0 {
		t.Fatalf("status=%d calls=%d", w.Code, sub.calls)
	}
}

func ptr(n int) *int { return &n }
