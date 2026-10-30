package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

const testPassword = "S3cr3t-Pa55-xyz"

type fakeService struct {
	registerFn func(service.RegisterInput) (domain.Router, error)
	getFn      func(id string) (domain.Router, error)
	listFn     func(limit, offset int) ([]domain.Router, int64, error)
	checkFn    func(id string) (domain.Router, error)
}

func (f *fakeService) Register(_ context.Context, _ coretenant.Scope, in service.RegisterInput) (domain.Router, error) {
	return f.registerFn(in)
}
func (f *fakeService) Get(_ context.Context, _ coretenant.Scope, id string) (domain.Router, error) {
	return f.getFn(id)
}
func (f *fakeService) List(_ context.Context, _ coretenant.Scope, limit, offset int) ([]domain.Router, int64, error) {
	return f.listFn(limit, offset)
}
func (f *fakeService) Check(_ context.Context, _ coretenant.Scope, id string) (domain.Router, error) {
	return f.checkFn(id)
}
func (f *fakeService) Update(context.Context, coretenant.Scope, string, service.UpdateInput) (domain.Router, error) {
	return domain.Router{}, nil
}
func (f *fakeService) Delete(context.Context, coretenant.Scope, string, string) error { return nil }

func sampleRouter() domain.Router {
	return domain.Router{
		ID: "22222222-2222-2222-2222-222222222222", Name: "edge-1", Host: "10.0.0.1", Port: 443,
		APIType: domain.APITypeREST, UseTLS: true, Username: "admin", PasswordEncrypted: "v1:ciphertext",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func routerFor(t *testing.T, svc RouterService) *gin.Engine {
	r := newTestRouter(t)
	NewRouterHandler(svc).RegisterRoutes(r.Group("/app/mikrotik"), nil, nil)
	return r
}

func errorBody(t *testing.T, body []byte) (code string, raw map[string]any) {
	t.Helper()
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	code, _ = raw["code"].(string)
	return code, raw
}

func TestCreateRouterReturns201WithoutPassword(t *testing.T) {
	var got service.RegisterInput
	r := routerFor(t, &fakeService{registerFn: func(in service.RegisterInput) (domain.Router, error) {
		got = in
		return sampleRouter(), nil
	}})
	w := performJSON(r, http.MethodPost, "/app/mikrotik/routers",
		fmt.Sprintf(`{"name":"edge-1","host":"10.0.0.1","port":443,"username":"admin","password":%q,"use_tls":true}`, testPassword))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
	if got.Password != testPassword || got.Host != "10.0.0.1" {
		t.Fatalf("service input = %+v", got)
	}
	if strings.Contains(w.Body.String(), testPassword) || strings.Contains(w.Body.String(), "ciphertext") ||
		strings.Contains(w.Body.String(), "password_encrypted") {
		t.Fatalf("response leaks password material: %s", w.Body)
	}
	if !strings.Contains(w.Body.String(), `"has_password":true`) {
		t.Fatalf("has_password missing: %s", w.Body)
	}
}

func TestCreateRouterValidation422(t *testing.T) {
	cases := map[string]string{
		"host empty":     `{"name":"a","host":"","port":443,"username":"u","password":"p"}`,
		"port zero":      `{"name":"a","host":"10.0.0.1","port":0,"username":"u","password":"p"}`,
		"username empty": `{"name":"a","host":"10.0.0.1","port":443,"username":"","password":"p"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			r := routerFor(t, &fakeService{registerFn: func(service.RegisterInput) (domain.Router, error) {
				t.Fatal("service must not be called")
				return domain.Router{}, nil
			}})
			w := performJSON(r, http.MethodPost, "/app/mikrotik/routers", body)
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body)
			}
			if code, _ := errorBody(t, w.Body.Bytes()); code != "VALIDATION_ERROR" {
				t.Fatalf("code = %q", code)
			}
		})
	}
}

func TestCreateRouterTLSUntrustedReturns422WithFingerprint(t *testing.T) {
	r := routerFor(t, &fakeService{registerFn: func(service.RegisterInput) (domain.Router, error) {
		return domain.Router{}, &mikrotik.TLSUntrustedError{Fingerprint: "abc123", PinMismatch: true}
	}})
	w := performJSON(r, http.MethodPost, "/app/mikrotik/routers",
		`{"name":"a","host":"10.0.0.1","port":443,"username":"u","password":"p","use_tls":true}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
	code, raw := errorBody(t, w.Body.Bytes())
	data, _ := raw["data"].(map[string]any)
	if code != "MIKROTIK_TLS_UNTRUSTED" || data["fingerprint"] != "abc123" || data["pin_mismatch"] != true {
		t.Fatalf("unexpected body: %s", w.Body)
	}
}

func TestGetRouterNotFound404(t *testing.T) {
	r := routerFor(t, &fakeService{getFn: func(string) (domain.Router, error) {
		return domain.Router{}, mikrotik.ErrRouterNotFound
	}})
	w := performJSON(r, http.MethodGet, "/app/mikrotik/routers/33333333-3333-3333-3333-333333333333", "")
	if code, _ := errorBody(t, w.Body.Bytes()); w.Code != http.StatusNotFound || code != "MIKROTIK_ROUTER_NOT_FOUND" {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
}

func TestListRoutersPaginates(t *testing.T) {
	var gotLimit, gotOffset int
	r := routerFor(t, &fakeService{listFn: func(limit, offset int) ([]domain.Router, int64, error) {
		gotLimit, gotOffset = limit, offset
		return []domain.Router{sampleRouter()}, 45, nil
	}})
	w := performJSON(r, http.MethodGet, "/app/mikrotik/routers?page=3&per_page=20", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
	if gotLimit != 20 || gotOffset != 40 {
		t.Fatalf("limit/offset = %d/%d", gotLimit, gotOffset)
	}
	var body struct {
		Data []map[string]any `json:"data"`
		Meta map[string]any   `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Meta["page"] != float64(3) || body.Meta["total"] != float64(45) || body.Meta["total_pages"] != float64(3) {
		t.Fatalf("unexpected body: %s", w.Body)
	}
}

func TestErrorMappingTable(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{mikrotik.ErrRouterNotFound, 404, "MIKROTIK_ROUTER_NOT_FOUND"},
		{mikrotik.ErrRouterNameTaken, 409, "MIKROTIK_ROUTER_NAME_TAKEN"},
		{&mikrotik.InvalidInputError{Message: "bad host"}, 422, "VALIDATION_ERROR"},
		{mikrotik.ErrHostNotAllowed, 422, "MIKROTIK_HOST_NOT_ALLOWED"},
		{mikrotik.ErrTLSRequired, 422, "MIKROTIK_TLS_REQUIRED"},
		{&mikrotik.TLSUntrustedError{Fingerprint: "ff"}, 422, "MIKROTIK_TLS_UNTRUSTED"},
		{mikrotik.ErrAuthFailed, 502, "MIKROTIK_AUTH_FAILED"},
		{mikrotik.ErrUnreachable, 502, "MIKROTIK_UNREACHABLE"},
		{fmt.Errorf("wrapped: %w", mikrotik.ErrRouterNotFound), 404, "MIKROTIK_ROUTER_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			failRouter(c, tc.err)
			if code, _ := errorBody(t, w.Body.Bytes()); w.Code != tc.status || code != tc.code {
				t.Fatalf("status/code = %d/%s, body = %s", w.Code, code, w.Body)
			}
		})
	}

	t.Run("unknown error is not echoed", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		failRouter(c, errors.New("dial 10.1.2.3 password=hunter2"))
		code, _ := errorBody(t, w.Body.Bytes())
		if w.Code != 500 || code != "INTERNAL_ERROR" || strings.Contains(w.Body.String(), "hunter2") {
			t.Fatalf("status/code = %d/%s, body = %s", w.Code, code, w.Body)
		}
	})
}

func TestCheckFailureReturnsMappedError(t *testing.T) {
	r := routerFor(t, &fakeService{checkFn: func(string) (domain.Router, error) {
		return sampleRouter(), mikrotik.ErrUnreachable
	}})
	w := performJSON(r, http.MethodPost, "/app/mikrotik/routers/33333333-3333-3333-3333-333333333333/check", "")
	if code, _ := errorBody(t, w.Body.Bytes()); w.Code != 502 || code != "MIKROTIK_UNREACHABLE" {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
}

// recordingChecker denies everything and records the permission each route asks for.
type recordingChecker struct{ asked []string }

func (c *recordingChecker) Can(_ context.Context, _ string, perms []string) error {
	c.asked = append(c.asked, perms...)
	return errors.New("denied")
}

func (c *recordingChecker) CanOrganization(_ context.Context, _, _ string, perms []string) error {
	c.asked = append(c.asked, perms...)
	return errors.New("denied")
}

func TestRoutesRequirePermissions(t *testing.T) {
	routes := []struct{ method, path, perm string }{
		{http.MethodGet, "/app/mikrotik/routers", "mikrotik_router.read"},
		{http.MethodPost, "/app/mikrotik/routers", "mikrotik_router.create"},
		{http.MethodGet, "/app/mikrotik/routers/:id", "mikrotik_router.read"},
		{http.MethodPatch, "/app/mikrotik/routers/:id", "mikrotik_router.update"},
		{http.MethodDelete, "/app/mikrotik/routers/:id", "mikrotik_router.delete"},
		{http.MethodPost, "/app/mikrotik/routers/:id/check", "mikrotik_router.update"},
	}

	engine := newTestRouter(t)
	NewRouterHandler(&fakeService{}).RegisterRoutes(engine.Group("/app/mikrotik"), nil, nil)
	registered := map[string]bool{}
	for _, ri := range engine.Routes() {
		registered[ri.Method+" "+ri.Path] = true
	}
	if len(registered) != len(routes) {
		t.Fatalf("registered %d routes, want %d: %v", len(registered), len(routes), registered)
	}
	for _, rt := range routes {
		if !registered[rt.method+" "+rt.path] {
			t.Errorf("route %s %s not registered", rt.method, rt.path)
		}
	}

	// With a checker wired, every route must consult it with the expected
	// permission and reject the request when denied (handler never reached).
	for _, rt := range routes {
		checker := &recordingChecker{}
		r := newTestRouter(t)
		r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "user-1") })
		NewRouterHandler(&fakeService{}).RegisterRoutes(r.Group("/app/mikrotik"), checker, nil)
		path := strings.ReplaceAll(rt.path, ":id", "33333333-3333-3333-3333-333333333333")
		w := performJSON(r, rt.method, path, `{}`)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", rt.method, rt.path, w.Code)
		}
		if len(checker.asked) == 0 || checker.asked[0] != rt.perm {
			t.Errorf("%s %s: asked %v, want %s", rt.method, rt.path, checker.asked, rt.perm)
		}
	}
}
