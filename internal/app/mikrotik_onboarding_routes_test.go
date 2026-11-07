package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	mikrotikhandler "zyad.cloud/internal/modules/mikrotik/handler"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
)

type countingOnboardingService struct{ calls int }

func (s *countingOnboardingService) Summary(context.Context, coretenant.Scope) (mikrotikservice.OnboardingSummary, error) {
	s.calls++
	return mikrotikservice.OnboardingSummary{}, nil
}
func (s *countingOnboardingService) CustomerStatuses(context.Context, coretenant.Scope, []string) ([]mikrotikservice.CustomerStatus, error) {
	s.calls++
	return nil, nil
}

var onboardingRouteCases = []secretRouteCase{
	{http.MethodGet, "/onboarding", ``, "mikrotik_router.read", false},
	{http.MethodGet, "/customers/status?account_ids=" + rtAccount, ``, "mikrotik_policy.read", false},
}

// memberRolePermissions mirrors the grants migration 000175 gives the seeded
// `member` role for Mikrotik (read-only task dashboard). The seeded value
// itself is asserted by the integration test TestMemberHasTaskRead.
var memberRolePermissions = []string{"mikrotik_task.read"}

// newMikrotikFullEngine wires every Mikrotik handler that exposes a route
// touched by the tenant UI, with the real Authenticate middleware.
func newMikrotikFullEngine(t *testing.T, entitled bool, granted []string) (*gin.Engine, *countingOnboardingService, *countingTaskService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	allowed := map[string]bool{}
	for _, p := range granted {
		allowed[p] = true
	}
	tc := mikrotikTenantContext(t, rtOrg)
	onb, tasks := &countingOnboardingService{}, &countingTaskService{}
	engine := gin.New()
	protected := engine.Group("/api/v1")
	protected.Use(
		middleware.Authenticate(fakeBearerAuthenticator{}),
		func(c *gin.Context) {
			c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
			c.Next()
		},
	)
	registerMikrotikRoutes(protected, Dependencies{
		CRMEntitlementChecker:     &recordingEntitlements{allow: entitled},
		PermissionChecker:         permissionSetChecker{allowed: allowed},
		MikrotikRouterHandler:     mikrotikhandler.NewRouterHandler(&countingRouterService{}),
		MikrotikSecretHandler:     mikrotikhandler.NewSecretHandler(&countingSecretService{}),
		MikrotikTaskHandler:       mikrotikhandler.NewTaskHandler(tasks),
		MikrotikOnboardingHandler: mikrotikhandler.NewOnboardingHandler(onb),
	})
	return engine, onb, tasks
}

func doFull(engine *gin.Engine, rc secretRouteCase, token bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(rc.method, "/api/v1/app/mikrotik"+rc.path, nil)
	if token {
		req.Header.Set("Authorization", "Bearer test-token")
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestMikrotikOnboardingRoutes_RegisteredAndGuarded(t *testing.T) {
	for _, rc := range onboardingRouteCases {
		// registered + permission granted => reaches the handler
		engine, onb, _ := newMikrotikFullEngine(t, true, []string{rc.perm})
		if w := doFull(engine, rc, true); w.Code != http.StatusOK || onb.calls != 1 {
			t.Errorf("%s with %s: status %d calls %d body %s", rc.path, rc.perm, w.Code, onb.calls, w.Body)
		}
		// no token => 401
		engine, onb, _ = newMikrotikFullEngine(t, true, []string{rc.perm})
		if w := doFull(engine, rc, false); w.Code != http.StatusUnauthorized || onb.calls != 0 {
			t.Errorf("%s without token: status %d calls %d", rc.path, w.Code, onb.calls)
		}
		// other permission only => 403
		engine, onb, _ = newMikrotikFullEngine(t, true, []string{"mikrotik_task.read"})
		if w := doFull(engine, rc, true); w.Code != http.StatusForbidden || onb.calls != 0 {
			t.Errorf("%s without %s: status %d calls %d", rc.path, rc.perm, w.Code, onb.calls)
		}
		// no entitlement => 403
		engine, onb, _ = newMikrotikFullEngine(t, false, []string{rc.perm})
		if w := doFull(engine, rc, true); w.Code != http.StatusForbidden || onb.calls != 0 {
			t.Errorf("%s without entitlement: status %d calls %d", rc.path, w.Code, onb.calls)
		}
	}
}

// A member only holds mikrotik_task.read: the dashboard reads work, retry and
// the router-level onboarding/status views are forbidden.
func TestMikrotikTaskRoutes_MemberIsReadOnly(t *testing.T) {
	engine, onb, tasks := newMikrotikFullEngine(t, true, memberRolePermissions)
	for _, rc := range taskRouteCases[:2] {
		if w := doFull(engine, rc, true); w.Code != http.StatusOK {
			t.Errorf("member %s %s: status %d, want 200 (body %s)", rc.method, rc.path, w.Code, w.Body)
		}
	}
	if tasks.calls != 2 {
		t.Fatalf("member reads should reach the task service twice, got %d", tasks.calls)
	}
	if w := doFull(engine, taskRouteCases[2], true); w.Code != http.StatusForbidden {
		t.Errorf("member POST retry: status %d, want 403", w.Code)
	}
	if tasks.calls != 2 {
		t.Errorf("retry reached the service for a member")
	}
	for _, rc := range onboardingRouteCases {
		if w := doFull(engine, rc, true); w.Code != http.StatusForbidden {
			t.Errorf("member %s: status %d, want 403", rc.path, w.Code)
		}
	}
	if onb.calls != 0 {
		t.Errorf("onboarding service reached by member")
	}
}

func TestMikrotikTaskRoutes_NoTokenIs401(t *testing.T) {
	engine, _, tasks := newMikrotikFullEngine(t, true, memberRolePermissions)
	for _, rc := range taskRouteCases {
		if w := doFull(engine, rc, false); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status %d, want 401", rc.method, rc.path, w.Code)
		}
	}
	if tasks.calls != 0 {
		t.Fatalf("service reached without token")
	}
}
