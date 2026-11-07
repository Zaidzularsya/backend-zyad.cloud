package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	mikrotikdomain "zyad.cloud/internal/modules/mikrotik/domain"
	mikrotikhandler "zyad.cloud/internal/modules/mikrotik/handler"
	mikrotikrepo "zyad.cloud/internal/modules/mikrotik/repository"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
)

type countingTaskService struct{ calls int }

func (s *countingTaskService) List(context.Context, coretenant.Scope, mikrotikrepo.TaskFilter, int, int) ([]mikrotikdomain.TaskView, int64, error) {
	s.calls++
	return nil, 0, nil
}
func (s *countingTaskService) Summary(context.Context, coretenant.Scope) (mikrotikservice.TaskSummary, error) {
	s.calls++
	return mikrotikservice.TaskSummary{}, nil
}
func (s *countingTaskService) Retry(context.Context, coretenant.Scope, string, string) (mikrotikdomain.TaskView, error) {
	s.calls++
	return mikrotikdomain.TaskView{}, nil
}

const rtTask = "77777777-7777-7777-7777-777777777777"

var taskRouteCases = []secretRouteCase{
	{http.MethodGet, "/tasks", ``, "mikrotik_task.read", false},
	{http.MethodGet, "/tasks/summary", ``, "mikrotik_task.read", false},
	{http.MethodPost, "/tasks/" + rtTask + "/retry", ``, "mikrotik_task.retry", true},
}

func newTaskRoutesEngine(t *testing.T, entitled bool, granted []string, counter middleware.RateCounter) (*gin.Engine, *countingTaskService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	allowed := map[string]bool{}
	for _, p := range granted {
		allowed[p] = true
	}
	tc := mikrotikTenantContext(t, rtOrg)
	svc := &countingTaskService{}
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
		CRMEntitlementChecker: &recordingEntitlements{allow: entitled},
		PermissionChecker:     permissionSetChecker{allowed: allowed},
		MikrotikRouterHandler: mikrotikhandler.NewRouterHandler(&countingRouterService{}),
		MikrotikTaskHandler:   mikrotikhandler.NewTaskHandler(svc),
		MikrotikRateCounter:   counter,
	})
	return engine, svc
}

func doTask(engine *gin.Engine, rc secretRouteCase) *httptest.ResponseRecorder {
	req := httptest.NewRequest(rc.method, "/api/v1/app/mikrotik"+rc.path, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestTaskRoutes_EntitlementGate(t *testing.T) {
	engine, svc := newTaskRoutesEngine(t, false, []string{"mikrotik_task.read", "mikrotik_task.retry"}, nil)
	for _, rc := range taskRouteCases {
		if w := doTask(engine, rc); w.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", rc.method, rc.path, w.Code)
		}
	}
	if svc.calls != 0 {
		t.Fatalf("service reached without entitlement: %d calls", svc.calls)
	}
}

func TestTaskRoutes_PermissionGateAndRetryRateLimit(t *testing.T) {
	for _, rc := range taskRouteCases {
		other := "mikrotik_task.retry"
		if rc.perm == other {
			other = "mikrotik_task.read"
		}
		denied, dsvc := newTaskRoutesEngine(t, true, []string{other}, nil)
		if w := doTask(denied, rc); w.Code != http.StatusForbidden || dsvc.calls != 0 {
			t.Errorf("%s %s without %s: status %d calls %d", rc.method, rc.path, rc.perm, w.Code, dsvc.calls)
		}
		granted, gsvc := newTaskRoutesEngine(t, true, []string{rc.perm}, nil)
		if w := doTask(granted, rc); w.Code >= 300 || gsvc.calls != 1 {
			t.Errorf("%s %s with %s: status %d calls %d body %s", rc.method, rc.path, rc.perm, w.Code, gsvc.calls, w.Body)
		}
	}
	retry := taskRouteCases[2]
	counter := &fakeMikrotikCounter{counts: map[string]int64{}}
	engine, svc := newTaskRoutesEngine(t, true, []string{"mikrotik_task.read", "mikrotik_task.retry"}, counter)
	for i := 0; i < mikrotikProbeLimit; i++ {
		doTask(engine, retry)
	}
	before := svc.calls
	if w := doTask(engine, retry); w.Code != http.StatusTooManyRequests || svc.calls != before {
		t.Fatalf("over limit: status %d", w.Code)
	}
}
