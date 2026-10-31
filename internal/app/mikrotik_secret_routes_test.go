package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	mikrotikdomain "zyad.cloud/internal/modules/mikrotik/domain"
	mikrotikhandler "zyad.cloud/internal/modules/mikrotik/handler"
	mikrotikrepo "zyad.cloud/internal/modules/mikrotik/repository"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
)

const (
	rtOrg     = "11111111-1111-1111-1111-111111111111"
	rtRouter  = "33333333-3333-3333-3333-333333333333"
	rtSecret  = "44444444-4444-4444-4444-444444444444"
	rtAccount = "55555555-5555-5555-5555-555555555555"
	rtProduct = "66666666-6666-6666-6666-666666666666"
)

// countingSecretService and countingPolicyService count how often a request
// got past every gate and reached the service.
type countingSecretService struct{ calls int }

func (s *countingSecretService) Import(context.Context, coretenant.Scope, string, string) (mikrotikrepo.ImportResult, error) {
	s.calls++
	return mikrotikrepo.ImportResult{}, nil
}
func (s *countingSecretService) List(context.Context, coretenant.Scope, mikrotikrepo.SecretFilter, int, int) ([]mikrotikdomain.PPPSecret, int64, error) {
	s.calls++
	return nil, 0, nil
}
func (s *countingSecretService) Pair(context.Context, coretenant.Scope, string, string, string) (mikrotikdomain.PPPSecret, error) {
	s.calls++
	return mikrotikdomain.PPPSecret{}, nil
}
func (s *countingSecretService) PairNewCustomer(context.Context, coretenant.Scope, string, string) (mikrotikdomain.PPPSecret, error) {
	s.calls++
	return mikrotikdomain.PPPSecret{}, nil
}
func (s *countingSecretService) Unpair(context.Context, coretenant.Scope, string, string) error {
	s.calls++
	return nil
}
func (s *countingSecretService) SetOriginalProfile(context.Context, coretenant.Scope, string, string, string) error {
	s.calls++
	return nil
}
func (s *countingSecretService) SearchAccounts(context.Context, coretenant.Scope, string, int) ([]mikrotikservice.AccountRef, error) {
	s.calls++
	return nil, nil
}

type countingPolicyService struct{ calls int }

func (s *countingPolicyService) SetExempt(context.Context, coretenant.Scope, string, bool, string, string) (mikrotikdomain.CustomerPolicy, error) {
	s.calls++
	return mikrotikdomain.CustomerPolicy{}, nil
}
func (s *countingPolicyService) GetPolicy(context.Context, coretenant.Scope, string) (mikrotikdomain.CustomerPolicy, error) {
	s.calls++
	return mikrotikdomain.CustomerPolicy{}, nil
}
func (s *countingPolicyService) PreviewTrigger(context.Context, coretenant.Scope, string) (mikrotikservice.TriggerPreview, error) {
	s.calls++
	return mikrotikservice.TriggerPreview{}, nil
}
func (s *countingPolicyService) SetTrigger(context.Context, coretenant.Scope, string, bool, string) (mikrotikdomain.ProductTrigger, error) {
	s.calls++
	return mikrotikdomain.ProductTrigger{}, nil
}
func (s *countingPolicyService) ListTriggersWithProducts(context.Context, coretenant.Scope) ([]mikrotikservice.TriggerItem, error) {
	s.calls++
	return nil, nil
}
func (s *countingPolicyService) UnmarkedProducts(context.Context, coretenant.Scope) ([]mikrotikservice.ProductInfo, error) {
	s.calls++
	return nil, nil
}

// permissionSetChecker grants exactly the permissions in allowed.
type permissionSetChecker struct{ allowed map[string]bool }

func (c permissionSetChecker) check(perms []string) error {
	for _, p := range perms {
		if !c.allowed[p] {
			return errors.New("denied")
		}
	}
	return nil
}
func (c permissionSetChecker) Can(_ context.Context, _ string, perms []string) error {
	return c.check(perms)
}
func (c permissionSetChecker) CanOrganization(_ context.Context, _, _ string, perms []string) error {
	return c.check(perms)
}

type fakeBearerAuthenticator struct{}

func (fakeBearerAuthenticator) AuthenticateAccessToken(context.Context, string) (middleware.AuthenticatedUser, error) {
	return middleware.AuthenticatedUser{ID: "user-1", SessionID: "session-1"}, nil
}

type secretRouteCase struct {
	method, path, body, perm string
	limited                  bool
}

var secretRouteCases = []secretRouteCase{
	{http.MethodPost, "/routers/" + rtRouter + "/secrets/import", ``, "mikrotik_secret.import", true},
	{http.MethodGet, "/secrets", ``, "mikrotik_secret.read", false},
	{http.MethodPost, "/secrets/" + rtSecret + "/pair", `{"account_id":"` + rtAccount + `"}`, "mikrotik_secret.pair", false},
	{http.MethodPost, "/secrets/" + rtSecret + "/pair-new-customer", ``, "mikrotik_secret.pair", false},
	{http.MethodPost, "/secrets/" + rtSecret + "/unpair", ``, "mikrotik_secret.unpair", false},
	{http.MethodPut, "/secrets/" + rtSecret + "/original-profile", `{"profile":"paket-10m"}`, "mikrotik_secret.pair", true},
	{http.MethodGet, "/accounts/search?q=alice", ``, "mikrotik_secret.read", false},
	{http.MethodGet, "/customers/" + rtAccount + "/policy", ``, "mikrotik_policy.read", false},
	{http.MethodPut, "/customers/" + rtAccount + "/policy", `{"exempt":false}`, "mikrotik_policy.update", false},
	{http.MethodGet, "/products/unmarked", ``, "mikrotik_product_trigger.read", false},
	{http.MethodGet, "/products/triggers", ``, "mikrotik_product_trigger.read", false},
	{http.MethodGet, "/products/" + rtProduct + "/trigger/preview", ``, "mikrotik_product_trigger.read", false},
	{http.MethodPut, "/products/" + rtProduct + "/trigger", `{"enabled":true}`, "mikrotik_product_trigger.update", false},
}

type secretRoutesEnv struct {
	engine  *gin.Engine
	secrets *countingSecretService
	policy  *countingPolicyService
}

func (e secretRoutesEnv) calls() int { return e.secrets.calls + e.policy.calls }

// newSecretRoutesEnv wires the real Authenticate middleware (so a missing token
// is a genuine 401) and a fixed tenant context.
func newSecretRoutesEnv(t *testing.T, entitled bool, granted []string, counter middleware.RateCounter) secretRoutesEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tc := mikrotikTenantContext(t, rtOrg)
	allowed := map[string]bool{}
	for _, p := range granted {
		allowed[p] = true
	}
	env := secretRoutesEnv{engine: gin.New(), secrets: &countingSecretService{}, policy: &countingPolicyService{}}
	protected := env.engine.Group("/api/v1")
	protected.Use(
		middleware.Authenticate(fakeBearerAuthenticator{}),
		func(c *gin.Context) {
			c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
			c.Next()
		},
	)
	registerMikrotikRoutes(protected, Dependencies{
		CRMEntitlementChecker: &recordingEntitlements{allow: entitled},
		PermissionChecker:     permissionSetChecker{allowed: allowed},
		MikrotikRouterHandler: mikrotikhandler.NewRouterHandler(&countingRouterService{}),
		MikrotikSecretHandler: mikrotikhandler.NewSecretHandler(env.secrets),
		MikrotikPolicyHandler: mikrotikhandler.NewPolicyHandler(env.policy),
		MikrotikRateCounter:   counter,
	})
	return env
}

func (e secretRoutesEnv) do(rc secretRouteCase, token bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(rc.method, "/api/v1/app/mikrotik"+rc.path, strings.NewReader(rc.body))
	req.Header.Set("Content-Type", "application/json")
	if token {
		req.Header.Set("Authorization", "Bearer test-token")
	}
	w := httptest.NewRecorder()
	e.engine.ServeHTTP(w, req)
	return w
}

func allSecretPermissions() []string {
	seen := map[string]bool{}
	var out []string
	for _, rc := range secretRouteCases {
		if !seen[rc.perm] {
			seen[rc.perm] = true
			out = append(out, rc.perm)
		}
	}
	return out
}

func TestMikrotikSecretRoutes_NoTokenIs401(t *testing.T) {
	env := newSecretRoutesEnv(t, true, allSecretPermissions(), nil)
	for _, rc := range secretRouteCases {
		if w := env.do(rc, false); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", rc.method, rc.path, w.Code)
		}
	}
	if env.calls() != 0 {
		t.Fatalf("service reached without a token: %d calls", env.calls())
	}
}

func TestMikrotikSecretRoutes_NoEntitlementIs403(t *testing.T) {
	env := newSecretRoutesEnv(t, false, allSecretPermissions(), nil)
	for _, rc := range secretRouteCases {
		if w := env.do(rc, true); w.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", rc.method, rc.path, w.Code)
		}
	}
	if env.calls() != 0 {
		t.Fatalf("service reached without entitlement: %d calls", env.calls())
	}
}

func TestMikrotikSecretRoutes_RoleWithoutPermissionIs403(t *testing.T) {
	for _, rc := range secretRouteCases {
		var others []string
		for _, p := range allSecretPermissions() {
			if p != rc.perm {
				others = append(others, p)
			}
		}
		denied := newSecretRoutesEnv(t, true, others, nil)
		if w := denied.do(rc, true); w.Code != http.StatusForbidden || denied.calls() != 0 {
			t.Errorf("%s %s without %s: status = %d, calls = %d, want 403/0", rc.method, rc.path, rc.perm, w.Code, denied.calls())
		}
		granted := newSecretRoutesEnv(t, true, []string{rc.perm}, nil)
		if w := granted.do(rc, true); w.Code >= 300 || granted.calls() != 1 {
			t.Errorf("%s %s with only %s: status = %d, calls = %d, want 2xx/1, body = %s", rc.method, rc.path, rc.perm, w.Code, granted.calls(), w.Body)
		}
	}
}

func TestMikrotikSecretRoutes_ImportAndOriginalProfileAreProbeLimited(t *testing.T) {
	var limited, free []secretRouteCase
	for _, rc := range secretRouteCases {
		if rc.limited {
			limited = append(limited, rc)
		} else {
			free = append(free, rc)
		}
	}
	if len(limited) != 2 {
		t.Fatalf("expected import and original-profile to be limited, got %v", limited)
	}
	for _, rc := range limited {
		counter := &fakeMikrotikCounter{counts: map[string]int64{}}
		env := newSecretRoutesEnv(t, true, allSecretPermissions(), counter)
		for i := 1; i <= mikrotikProbeLimit; i++ {
			if w := env.do(rc, true); w.Code >= 300 {
				t.Fatalf("%s request %d: status %d body %s", rc.path, i, w.Code, w.Body)
			}
		}
		before := env.calls()
		w := env.do(rc, true)
		if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), `"code":"RATE_LIMITED"`) {
			t.Fatalf("%s over the limit: status %d body %s, want 429 RATE_LIMITED", rc.path, w.Code, w.Body)
		}
		if env.calls() != before {
			t.Fatalf("%s: limited request reached the service", rc.path)
		}
		// The window is shared across router-contacting routes, so the other limited route is blocked too.
		for _, other := range limited {
			if w := env.do(other, true); w.Code != http.StatusTooManyRequests {
				t.Errorf("%s after limit: status %d, want 429", other.path, w.Code)
			}
		}
		// Routes that never contact the router stay available.
		for _, f := range free {
			if w := env.do(f, true); w.Code == http.StatusTooManyRequests {
				t.Errorf("%s %s must not be rate limited", f.method, f.path)
			}
		}
	}
}
