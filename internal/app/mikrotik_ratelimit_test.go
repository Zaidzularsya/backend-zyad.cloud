package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"

	coretenant "zyad.cloud/internal/core/tenant"
	mikrotikhandler "zyad.cloud/internal/modules/mikrotik/handler"
)

type fakeMikrotikCounter struct {
	mu     sync.Mutex
	counts map[string]int64
}

func (f *fakeMikrotikCounter) Incr(ctx context.Context, key string) *goredis.IntCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[key]++
	cmd := goredis.NewIntCmd(ctx)
	cmd.SetVal(f.counts[key])
	return cmd
}

func (f *fakeMikrotikCounter) Expire(ctx context.Context, _ string, _ time.Duration) *goredis.BoolCmd {
	return goredis.NewBoolCmd(ctx)
}

func mikrotikTenantContext(t *testing.T, orgID string) coretenant.Context {
	t.Helper()
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: orgID, OrganizationSlug: "org-" + orgID[:4],
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", MembershipStatus: "active", MembershipVersion: 1,
		ResolutionSource: coretenant.ResolutionSourceSession, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tc
}

// rateLimitedEngine resolves the tenant from the X-Test-Org header so one
// engine can serve several organizations.
func rateLimitedEngine(t *testing.T, counter *fakeMikrotikCounter, allow bool) (*gin.Engine, *countingRouterService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := &countingRouterService{}
	engine := gin.New()
	protected := engine.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		tc := mikrotikTenantContext(t, c.GetHeader("X-Test-Org"))
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
		c.Next()
	})
	registerMikrotikRoutes(protected, Dependencies{
		CRMEntitlementChecker: &recordingEntitlements{allow: allow},
		MikrotikRouterHandler: mikrotikhandler.NewRouterHandler(svc),
		MikrotikSetupHandler:  mikrotikhandler.NewSetupHandler(&countingSetupService{}),
		MikrotikRateCounter:   counter,
	})
	return engine, svc
}

func mikrotikRequest(engine http.Handler, method, path, org string) *httptest.ResponseRecorder {
	body := ""
	if method == http.MethodPost && strings.HasSuffix(path, "/routers") {
		body = `{"name":"a","host":"10.0.0.1","port":8728,"username":"u","password":"p"}`
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Org", org)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestMikrotikRouterCallsAreRateLimitedPerOrganization(t *testing.T) {
	const orgA = "11111111-1111-1111-1111-111111111111"
	const orgB = "22222222-2222-2222-2222-222222222222"
	counter := &fakeMikrotikCounter{counts: map[string]int64{}}
	engine, svc := rateLimitedEngine(t, counter, true)
	const routers = "/api/v1/app/mikrotik/routers"

	for i := 1; i <= mikrotikProbeLimit; i++ {
		if w := mikrotikRequest(engine, http.MethodPost, routers, orgA); w.Code != http.StatusCreated {
			t.Fatalf("request %d: status %d body %s", i, w.Code, w.Body)
		}
	}
	w := mikrotikRequest(engine, http.MethodPost, routers, orgA)
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), `"code":"RATE_LIMITED"`) {
		t.Fatalf("request %d: status %d body %s, want 429 RATE_LIMITED", mikrotikProbeLimit+1, w.Code, w.Body)
	}
	if svc.calls != mikrotikProbeLimit {
		t.Fatalf("service calls = %d, want %d (limited request must not reach the handler)", svc.calls, mikrotikProbeLimit)
	}
	// The window is shared by every router-contacting route of the organization.
	if w := mikrotikRequest(engine, http.MethodPost, routers+"/33333333-3333-3333-3333-333333333333/check", orgA); w.Code != http.StatusTooManyRequests {
		t.Fatalf("check after limit: status %d", w.Code)
	}
	if w := mikrotikRequest(engine, http.MethodPost, routers+"/33333333-3333-3333-3333-333333333333/setup/cleanup", orgA); w.Code != http.StatusTooManyRequests {
		t.Fatalf("setup after limit: status %d", w.Code)
	}

	// Reads (and delete) are not limited.
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, routers},
		{http.MethodGet, routers + "/33333333-3333-3333-3333-333333333333"},
		{http.MethodDelete, routers + "/33333333-3333-3333-3333-333333333333"},
	} {
		if w := mikrotikRequest(engine, rt.method, rt.path, orgA); w.Code == http.StatusTooManyRequests {
			t.Fatalf("%s %s must not be rate limited", rt.method, rt.path)
		}
	}

	// Another organization has its own counter.
	if w := mikrotikRequest(engine, http.MethodPost, routers, orgB); w.Code != http.StatusCreated {
		t.Fatalf("org B: status %d body %s", w.Code, w.Body)
	}
}

func TestMikrotikRateLimitRunsAfterEntitlementGate(t *testing.T) {
	counter := &fakeMikrotikCounter{counts: map[string]int64{}}
	engine, _ := rateLimitedEngine(t, counter, false)
	w := mikrotikRequest(engine, http.MethodPost, "/api/v1/app/mikrotik/routers", "11111111-1111-1111-1111-111111111111")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status %d", w.Code)
	}
	if len(counter.counts) != 0 {
		t.Fatalf("rate counter consumed before the entitlement gate: %v", counter.counts)
	}
}

func TestMikrotikRateLimitFailsOpenWithoutCounter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &countingRouterService{}
	engine := gin.New()
	protected := engine.Group("/api/v1")
	tc := mikrotikTenantContext(t, "11111111-1111-1111-1111-111111111111")
	protected.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
		c.Next()
	})
	registerMikrotikRoutes(protected, Dependencies{
		CRMEntitlementChecker: &recordingEntitlements{allow: true},
		MikrotikRouterHandler: mikrotikhandler.NewRouterHandler(svc),
	})
	for i := 0; i < mikrotikProbeLimit+5; i++ {
		if w := mikrotikRequest(engine, http.MethodPost, "/api/v1/app/mikrotik/routers", tc.OrganizationID()); w.Code != http.StatusCreated {
			t.Fatalf("request %d: status %d", i, w.Code)
		}
	}
}
