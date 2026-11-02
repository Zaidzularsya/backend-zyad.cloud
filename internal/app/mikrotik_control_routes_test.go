package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	mikrotikhandler "zyad.cloud/internal/modules/mikrotik/handler"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
)

type countingControlService struct{ calls int }

func (s *countingControlService) State(context.Context, coretenant.Scope) (mikrotikservice.ControlState, error) {
	s.calls++
	return mikrotikservice.ControlState{}, nil
}
func (s *countingControlService) SetGraceDays(context.Context, coretenant.Scope, int, string) (mikrotikservice.ControlState, error) {
	s.calls++
	return mikrotikservice.ControlState{}, nil
}
func (s *countingControlService) SetMode(context.Context, coretenant.Scope, mikrotikservice.SetModeInput, string) (mikrotikservice.ControlState, error) {
	s.calls++
	return mikrotikservice.ControlState{}, nil
}
func (s *countingControlService) ListPilot(context.Context, coretenant.Scope) ([]mikrotikservice.PilotView, error) {
	s.calls++
	return nil, nil
}
func (s *countingControlService) AddPilot(context.Context, coretenant.Scope, string, string) error {
	s.calls++
	return nil
}
func (s *countingControlService) RemovePilot(context.Context, coretenant.Scope, string, string) error {
	s.calls++
	return nil
}
func (s *countingControlService) StartPilotTest(context.Context, coretenant.Scope, string, string) (time.Time, error) {
	s.calls++
	return time.Now(), nil
}
func (s *countingControlService) StopPilotTest(context.Context, coretenant.Scope, string, string) error {
	s.calls++
	return nil
}
func (s *countingControlService) RestoreAll(context.Context, coretenant.Scope, string) (mikrotikservice.RestoreAllResult, error) {
	s.calls++
	return mikrotikservice.RestoreAllResult{}, nil
}

var controlRouteCases = []secretRouteCase{
	{http.MethodGet, "/control", ``, "mikrotik_control.read", false},
	{http.MethodGet, "/control/pilot", ``, "mikrotik_control.read", false},
	{http.MethodPut, "/control/grace-days", `{"grace_days":3}`, "mikrotik_control.update", false},
	{http.MethodPut, "/control/mode", `{"mode":"paused"}`, "mikrotik_control.update", false},
	{http.MethodPut, "/control/pilot/" + rtAccount, ``, "mikrotik_control.update", false},
	{http.MethodDelete, "/control/pilot/" + rtAccount, ``, "mikrotik_control.update", false},
	{http.MethodPost, "/control/pilot/" + rtAccount + "/test", ``, "mikrotik_control.update", false},
	{http.MethodDelete, "/control/pilot/" + rtAccount + "/test", ``, "mikrotik_control.update", false},
	{http.MethodPost, "/control/restore-all", ``, "mikrotik_control.restore", false},
	{http.MethodGet, "/control/preview", ``, "mikrotik_control.read", false},
	{http.MethodGet, "/control/preview/summary", ``, "mikrotik_control.read", false},
	{http.MethodGet, "/control/preview/export", ``, "mikrotik_control.read", false},
	{http.MethodGet, "/control/activity", ``, "mikrotik_control.read", false},
}

// countingPreviewService shares the control service call counter.
type countingPreviewService struct{ c *countingControlService }

func (s countingPreviewService) Summary(context.Context, coretenant.Scope) (mikrotikservice.CandidateSummary, error) {
	s.c.calls++
	return mikrotikservice.CandidateSummary{}, nil
}
func (s countingPreviewService) List(context.Context, coretenant.Scope, mikrotikservice.CandidateFilter, int, int) ([]mikrotikservice.CandidateRow, int64, error) {
	s.c.calls++
	return nil, 0, nil
}
func (s countingPreviewService) ExportCSV(_ context.Context, _ coretenant.Scope, _ mikrotikservice.CandidateFilter, w io.Writer, begin func(int64)) error {
	s.c.calls++
	begin(0)
	_, err := io.WriteString(w, "Akun\n")
	return err
}

// countingActivityService shares the control service call counter.
type countingActivityService struct{ c *countingControlService }

func (s countingActivityService) List(context.Context, coretenant.Scope, int, int) ([]mikrotikservice.ActivityItem, int64, error) {
	s.c.calls++
	return nil, 0, nil
}

// Grants of migrations 000175/000177: member reads, organization_owner all.
var (
	memberControlPermissions = []string{"mikrotik_task.read", "mikrotik_control.read"}
	ownerControlPermissions  = []string{"mikrotik_control.read", "mikrotik_control.update", "mikrotik_control.restore"}
)

func newMikrotikControlEngine(t *testing.T, entitled bool, granted []string) (*gin.Engine, *countingControlService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	allowed := map[string]bool{}
	for _, p := range granted {
		allowed[p] = true
	}
	tc := mikrotikTenantContext(t, rtOrg)
	svc := &countingControlService{}
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
		CRMEntitlementChecker:   &recordingEntitlements{allow: entitled},
		PermissionChecker:       permissionSetChecker{allowed: allowed},
		MikrotikRouterHandler:   mikrotikhandler.NewRouterHandler(&countingRouterService{}),
		MikrotikControlHandler:  mikrotikhandler.NewControlHandler(svc),
		MikrotikPreviewHandler:  mikrotikhandler.NewPreviewHandler(countingPreviewService{c: svc}),
		MikrotikActivityHandler: mikrotikhandler.NewActivityHandler(countingActivityService{c: svc}),
	})
	return engine, svc
}

func doControl(engine *gin.Engine, rc secretRouteCase) *httptest.ResponseRecorder {
	req := httptest.NewRequest(rc.method, "/api/v1/app/mikrotik"+rc.path, strings.NewReader(rc.body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestControlRoutes_Permissions(t *testing.T) {
	for _, rc := range controlRouteCases {
		read := rc.perm == "mikrotik_control.read"

		engine, svc := newMikrotikControlEngine(t, true, memberControlPermissions)
		w := doControl(engine, rc)
		if read && (w.Code != http.StatusOK || svc.calls != 1) {
			t.Errorf("member %s %s: status %d calls %d, want 200", rc.method, rc.path, w.Code, svc.calls)
		}
		if !read && (w.Code != http.StatusForbidden || svc.calls != 0) {
			t.Errorf("member %s %s: status %d calls %d, want 403", rc.method, rc.path, w.Code, svc.calls)
		}

		engine, svc = newMikrotikControlEngine(t, true, ownerControlPermissions)
		w = doControl(engine, rc)
		if (w.Code != http.StatusOK && w.Code != http.StatusNoContent && w.Code != http.StatusAccepted) || svc.calls != 1 {
			t.Errorf("owner %s %s: status %d calls %d body %s", rc.method, rc.path, w.Code, svc.calls, w.Body)
		}

		// No bearer token: rejected before any gate.
		engine, svc = newMikrotikControlEngine(t, true, ownerControlPermissions)
		req := httptest.NewRequest(rc.method, "/api/v1/app/mikrotik"+rc.path, strings.NewReader(rc.body))
		nw := httptest.NewRecorder()
		engine.ServeHTTP(nw, req)
		if nw.Code != http.StatusUnauthorized || svc.calls != 0 {
			t.Errorf("no token %s %s: status %d calls %d", rc.method, rc.path, nw.Code, svc.calls)
		}

		// The mikrotik.enabled entitlement gates the whole group.
		engine, svc = newMikrotikControlEngine(t, false, ownerControlPermissions)
		if w := doControl(engine, rc); w.Code != http.StatusForbidden || svc.calls != 0 {
			t.Errorf("unentitled %s %s: status %d", rc.method, rc.path, w.Code)
		}
	}
}

func TestActivityRoute_ReadPermission(t *testing.T) {
	rc := secretRouteCase{http.MethodGet, "/control/activity", ``, "mikrotik_control.read", false}
	engine, svc := newMikrotikControlEngine(t, true, memberControlPermissions)
	if w := doControl(engine, rc); w.Code != http.StatusOK || svc.calls != 1 {
		t.Errorf("member: status %d calls %d", w.Code, svc.calls)
	}
	engine, svc = newMikrotikControlEngine(t, true, nil)
	if w := doControl(engine, rc); w.Code != http.StatusForbidden || svc.calls != 0 {
		t.Errorf("no permission: status %d calls %d", w.Code, svc.calls)
	}
	engine, svc = newMikrotikControlEngine(t, true, memberControlPermissions)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/app/mikrotik/control/activity", nil)
	nw := httptest.NewRecorder()
	engine.ServeHTTP(nw, req)
	if nw.Code != http.StatusUnauthorized || svc.calls != 0 {
		t.Errorf("no token: status %d calls %d", nw.Code, svc.calls)
	}
}

// Control, preview and activity routes only touch the database (tasks contact
// routers), so they must not consume the router probe budget.
func TestControlRoutes_NotBehindProbeRateLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const org = "11111111-1111-1111-1111-111111111111"
	counter := &fakeMikrotikCounter{counts: map[string]int64{}}
	svc := &countingControlService{}
	engine := gin.New()
	protected := engine.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), mikrotikTenantContext(t, org)))
		c.Next()
	})
	registerMikrotikRoutes(protected, Dependencies{
		CRMEntitlementChecker:   &recordingEntitlements{allow: true},
		PermissionChecker:       permissionSetChecker{allowed: map[string]bool{"mikrotik_control.read": true, "mikrotik_control.update": true, "mikrotik_control.restore": true}},
		MikrotikRouterHandler:   mikrotikhandler.NewRouterHandler(&countingRouterService{}),
		MikrotikControlHandler:  mikrotikhandler.NewControlHandler(svc),
		MikrotikPreviewHandler:  mikrotikhandler.NewPreviewHandler(countingPreviewService{c: svc}),
		MikrotikActivityHandler: mikrotikhandler.NewActivityHandler(countingActivityService{c: svc}),
		MikrotikRateCounter:     counter,
	})
	for _, rc := range controlRouteCases {
		for i := 0; i <= mikrotikProbeLimit+2; i++ {
			if w := doControl(engine, rc); w.Code == http.StatusTooManyRequests {
				t.Fatalf("%s %s: rate limited at request %d", rc.method, rc.path, i+1)
			}
		}
	}
	if len(counter.counts) != 0 {
		t.Errorf("probe counter was touched: %v", counter.counts)
	}
}
