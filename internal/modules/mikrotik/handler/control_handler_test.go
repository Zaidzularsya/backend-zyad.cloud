package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/service"
)

type fakeControlService struct {
	err   error
	calls int
	mode  service.SetModeInput
	grace int
}

func (f *fakeControlService) State(context.Context, coretenant.Scope) (service.ControlState, error) {
	f.calls++
	return service.ControlState{Mode: domain.ModePilot, Engine: service.ModeOn, EffectiveMode: domain.ModePilot}, f.err
}
func (f *fakeControlService) SetGraceDays(_ context.Context, _ coretenant.Scope, days int, _ string) (service.ControlState, error) {
	f.calls++
	f.grace = days
	return service.ControlState{GraceDays: days}, f.err
}
func (f *fakeControlService) SetMode(_ context.Context, _ coretenant.Scope, in service.SetModeInput, _ string) (service.ControlState, error) {
	f.calls++
	f.mode = in
	return service.ControlState{Mode: in.Mode}, f.err
}
func (f *fakeControlService) ListPilot(context.Context, coretenant.Scope) ([]service.PilotView, error) {
	f.calls++
	return []service.PilotView{{PilotAccount: domain.PilotAccount{AccountID: testAccountID}, AccountName: "Budi"}}, f.err
}
func (f *fakeControlService) AddPilot(context.Context, coretenant.Scope, string, string) error {
	f.calls++
	return f.err
}
func (f *fakeControlService) RemovePilot(context.Context, coretenant.Scope, string, string) error {
	f.calls++
	return f.err
}
func (f *fakeControlService) StartPilotTest(context.Context, coretenant.Scope, string, string) (time.Time, error) {
	f.calls++
	return time.Date(2026, 11, 2, 3, 10, 0, 0, time.UTC), f.err
}
func (f *fakeControlService) StopPilotTest(context.Context, coretenant.Scope, string, string) error {
	f.calls++
	return f.err
}
func (f *fakeControlService) RestoreAll(context.Context, coretenant.Scope, string) (service.RestoreAllResult, error) {
	f.calls++
	return service.RestoreAllResult{Enqueued: 3, Skipped: 1}, f.err
}

const controlBase = "/app/mikrotik/control"

var controlRoutes = []struct{ method, path, body, perm string }{
	{http.MethodGet, controlBase, ``, "mikrotik_control.read"},
	{http.MethodPut, controlBase + "/grace-days", `{"grace_days":3}`, "mikrotik_control.update"},
	{http.MethodPut, controlBase + "/mode", `{"mode":"paused"}`, "mikrotik_control.update"},
	{http.MethodGet, controlBase + "/pilot", ``, "mikrotik_control.read"},
	{http.MethodPut, controlBase + "/pilot/:account_id", ``, "mikrotik_control.update"},
	{http.MethodDelete, controlBase + "/pilot/:account_id", ``, "mikrotik_control.update"},
	{http.MethodPost, controlBase + "/pilot/:account_id/test", ``, "mikrotik_control.update"},
	{http.MethodDelete, controlBase + "/pilot/:account_id/test", ``, "mikrotik_control.update"},
	{http.MethodPost, controlBase + "/restore-all", ``, "mikrotik_control.restore"},
}

func controlRouter(t *testing.T, svc ControlService) *gin.Engine {
	r := newTestRouter(t)
	NewControlHandler(svc).RegisterRoutes(r.Group("/app/mikrotik"), nil, nil)
	return r
}

func TestControlRoutes_RequirePermission(t *testing.T) {
	engine := controlRouter(t, &fakeControlService{})
	if n := len(engine.Routes()); n != len(controlRoutes) {
		t.Fatalf("registered %d routes, want %d", n, len(controlRoutes))
	}
	for _, rt := range controlRoutes {
		checker := &recordingChecker{}
		svc := &fakeControlService{}
		r := newTestRouter(t)
		r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "user-1") })
		NewControlHandler(svc).RegisterRoutes(r.Group("/app/mikrotik"), checker, nil)
		w := performJSON(r, rt.method, strings.ReplaceAll(rt.path, ":account_id", testAccountID), rt.body)
		if w.Code != http.StatusForbidden || svc.calls != 0 {
			t.Errorf("%s %s: status = %d calls = %d, want 403", rt.method, rt.path, w.Code, svc.calls)
		}
		if len(checker.asked) == 0 || checker.asked[0] != rt.perm {
			t.Errorf("%s %s: asked %v, want %s", rt.method, rt.path, checker.asked, rt.perm)
		}
	}
}

func TestControlRoutes_Success(t *testing.T) {
	want := map[string]int{
		http.MethodGet + " " + controlBase:                                http.StatusOK,
		http.MethodPut + " " + controlBase + "/grace-days":                http.StatusOK,
		http.MethodPut + " " + controlBase + "/mode":                      http.StatusOK,
		http.MethodGet + " " + controlBase + "/pilot":                     http.StatusOK,
		http.MethodPut + " " + controlBase + "/pilot/:account_id":         http.StatusNoContent,
		http.MethodDelete + " " + controlBase + "/pilot/:account_id":      http.StatusNoContent,
		http.MethodPost + " " + controlBase + "/pilot/:account_id/test":   http.StatusAccepted,
		http.MethodDelete + " " + controlBase + "/pilot/:account_id/test": http.StatusNoContent,
		http.MethodPost + " " + controlBase + "/restore-all":              http.StatusOK,
	}
	for _, rt := range controlRoutes {
		svc := &fakeControlService{}
		w := performJSON(controlRouter(t, svc), rt.method, strings.ReplaceAll(rt.path, ":account_id", testAccountID), rt.body)
		if w.Code != want[rt.method+" "+rt.path] || svc.calls != 1 {
			t.Errorf("%s %s: status = %d calls = %d body = %s", rt.method, rt.path, w.Code, svc.calls, w.Body)
		}
	}
}

func TestControlRoutes_Bodies(t *testing.T) {
	svc := &fakeControlService{}
	r := controlRouter(t, svc)
	w := performJSON(r, http.MethodPut, controlBase+"/mode",
		`{"mode":"active","exemptions":{"account_ids":["`+testAccountID+`"],"reason":"VIP","until":"2026-12-31"}}`)
	if w.Code != http.StatusOK || svc.mode.Mode != "active" || svc.mode.Exemptions == nil ||
		svc.mode.Exemptions.Reason != "VIP" || svc.mode.Exemptions.Until.Format("2006-01-02") != "2026-12-31" {
		t.Fatalf("status = %d in = %+v", w.Code, svc.mode)
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	w = performJSON(r, http.MethodPost, controlBase+"/pilot/"+testAccountID+"/test", ``)
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Data["test_until"] != "2026-11-02T03:10:00Z" {
		t.Fatalf("test body = %s", w.Body)
	}
	w = performJSON(r, http.MethodPost, controlBase+"/restore-all", ``)
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Data["enqueued"] != float64(3) || body.Data["skipped"] != float64(1) || body.Data["failed"] != float64(0) || body.Data["aborted"] != false {
		t.Fatalf("restore body = %s", w.Body)
	}
	w = performJSON(r, http.MethodGet, controlBase, ``)
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Data["effective_mode"] != "pilot" || body.Data["engine"] != "on" {
		t.Fatalf("state body = %s", w.Body)
	}

	// Validation at the boundary: never reaches the service.
	svc.calls = 0
	for _, tc := range []struct{ path, body string }{
		{"/mode", `{"mode":"active","extra":1}`},
		{"/mode", `{"mode":"active","exemptions":{"account_ids":["x"],"reason":"r","until":"31-12-2026"}}`},
		{"/grace-days", `{}`},
	} {
		if w := performJSON(r, http.MethodPut, controlBase+tc.path, tc.body); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s %s: status = %d", tc.path, tc.body, w.Code)
		}
	}
	if w := performJSON(r, http.MethodPut, controlBase+"/mode", `{"mode":"active","exemptions":{"account_ids":["not-a-uuid"],"reason":"r"}}`); w.Code != http.StatusNotFound {
		t.Errorf("malformed account id: status = %d", w.Code)
	}
	if w := performJSON(r, http.MethodPut, controlBase+"/pilot/not-a-uuid", ``); w.Code != http.StatusNotFound {
		t.Errorf("malformed path id: status = %d", w.Code)
	}
	if svc.calls != 0 {
		t.Fatalf("invalid requests reached the service %d times", svc.calls)
	}
}

func TestControlRoutes_ErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err    error
		method string
		path   string
		status int
		code   string
	}{
		{mikrotik.ErrPilotRequired, http.MethodPut, "/mode", http.StatusConflict, "MIKROTIK_PILOT_REQUIRED"},
		{errors.Join(mikrotik.ErrModeInvalid), http.MethodPut, "/mode", http.StatusUnprocessableEntity, "MIKROTIK_MODE_INVALID"},
		{&mikrotik.InvalidInputError{Message: "at most 500"}, http.MethodPut, "/mode", http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		{mikrotik.ErrAccountNotFound, http.MethodPut, "/mode", http.StatusNotFound, "MIKROTIK_ACCOUNT_NOT_FOUND"},
		{mikrotik.ErrEngineOff, http.MethodPost, "/pilot/" + testAccountID + "/test", http.StatusConflict, "MIKROTIK_ENGINE_OFF"},
		{mikrotik.ErrPilotModeRequired, http.MethodPost, "/pilot/" + testAccountID + "/test", http.StatusConflict, "MIKROTIK_PILOT_MODE_REQUIRED"},
		{mikrotik.ErrNotPaused, http.MethodPost, "/restore-all", http.StatusConflict, "MIKROTIK_NOT_PAUSED"},
		{mikrotik.ErrEngineOff, http.MethodPost, "/restore-all", http.StatusConflict, "MIKROTIK_ENGINE_OFF"},
		{&mikrotik.PilotTestNotRunnableError{Reason: mikrotik.PilotBlockedExempt}, http.MethodPost, "/pilot/" + testAccountID + "/test", http.StatusConflict, "MIKROTIK_PILOT_TEST_NOT_RUNNABLE"},
	} {
		body := ``
		if tc.method == http.MethodPut {
			body = `{"mode":"active"}`
		}
		w := performJSON(controlRouter(t, &fakeControlService{err: tc.err}), tc.method, controlBase+tc.path, body)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Errorf("%v: status = %d body = %s", tc.err, w.Code, w.Body)
		}
	}
}
