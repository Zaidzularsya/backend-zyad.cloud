package handler

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	corecrypto "zyad.cloud/internal/core/crypto"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
	"zyad.cloud/internal/modules/mikrotik/service"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

const (
	setupRouterID = "33333333-3333-3333-3333-333333333333"
	setupBase     = "/app/mikrotik/routers/" + setupRouterID + "/setup"
	setupBody     = `{"pool_cidr":"10.255.254.0/24","platform_host":"zyad.online","tenant_isolir_host":"t.example.com","doku_hosts":["sandbox.doku.com"]}`
	hash64        = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

type fakeSetupService struct {
	previewFn func(service.SetupInput) (domain.SetupPlan, error)
	applyFn   func(in service.SetupInput, hash string) (domain.Router, error)
	cleanupFn func() (domain.Router, error)
}

func (f *fakeSetupService) Preview(_ context.Context, _ coretenant.Scope, _ string, in service.SetupInput) (domain.SetupPlan, error) {
	return f.previewFn(in)
}
func (f *fakeSetupService) Apply(_ context.Context, _ coretenant.Scope, _ string, in service.SetupInput, hash, _ string) (domain.Router, error) {
	return f.applyFn(in, hash)
}
func (f *fakeSetupService) Cleanup(context.Context, coretenant.Scope, string, string) (domain.Router, error) {
	return f.cleanupFn()
}

func setupRouterFor(t *testing.T, svc SetupService) *gin.Engine {
	r := newTestRouter(t)
	NewSetupHandler(svc).RegisterRoutes(r.Group("/app/mikrotik"), nil, nil)
	return r
}

func TestApplyStaleHash409(t *testing.T) {
	r := setupRouterFor(t, &fakeSetupService{applyFn: func(service.SetupInput, string) (domain.Router, error) {
		return domain.Router{}, mikrotik.ErrPlanChanged
	}})
	body := strings.TrimSuffix(setupBody, "}") + `,"plan_hash":"` + hash64 + `"}`
	w := performJSON(r, http.MethodPost, setupBase+"/apply", body)
	if code, _ := errorBody(t, w.Body.Bytes()); w.Code != 409 || code != "MIKROTIK_SETUP_PLAN_CHANGED" {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
}

func TestApplyRequiresPlanHash(t *testing.T) {
	r := setupRouterFor(t, &fakeSetupService{})
	w := performJSON(r, http.MethodPost, setupBase+"/apply", setupBody)
	if code, _ := errorBody(t, w.Body.Bytes()); w.Code != 422 || code != "VALIDATION_ERROR" {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
}

func TestSetupConflict409(t *testing.T) {
	r := setupRouterFor(t, &fakeSetupService{previewFn: func(service.SetupInput) (domain.SetupPlan, error) {
		return domain.SetupPlan{}, &mikrotik.SetupConflictError{Kind: "ppp profile", Name: "zyad-isolir", Detail: "not managed by zyad"}
	}})
	w := performJSON(r, http.MethodPost, setupBase+"/preview", setupBody)
	code, raw := errorBody(t, w.Body.Bytes())
	if w.Code != 409 || code != "MIKROTIK_SETUP_CONFLICT" || !strings.Contains(w.Body.String(), "zyad-isolir") {
		t.Fatalf("status = %d, body = %s (%v)", w.Code, w.Body, raw)
	}
}

func TestSetupValidation422(t *testing.T) {
	r := setupRouterFor(t, &fakeSetupService{previewFn: func(service.SetupInput) (domain.SetupPlan, error) {
		return domain.SetupPlan{}, &mikrotik.InvalidInputError{Message: "pool_cidr overlaps an existing address pool on the router"}
	}})
	w := performJSON(r, http.MethodPost, setupBase+"/preview", setupBody)
	if code, _ := errorBody(t, w.Body.Bytes()); w.Code != 422 || code != "VALIDATION_ERROR" || !strings.Contains(w.Body.String(), "overlaps") {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
	// Malformed JSON is also a validation error, not a 500.
	w = performJSON(r, http.MethodPost, setupBase+"/preview", `{"pool_cidr":`)
	if w.Code != 422 {
		t.Fatalf("malformed body status = %d", w.Code)
	}
}

func TestSetupErrorMapping(t *testing.T) {
	step := &mikrotik.SetupStepError{Step: "create /ip/firewall/filter", Cause: mikrotik.ErrSetupFailed, RouterWritten: true}
	for name, tc := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"step failed":     {step, 502, "MIKROTIK_SETUP_FAILED"},
		"unreachable":     {&mikrotik.SetupStepError{Step: "create /ip/pool", Cause: mikrotik.ErrUnreachable}, 502, "MIKROTIK_UNREACHABLE"},
		"auth":            {mikrotik.ErrAuthFailed, 502, "MIKROTIK_AUTH_FAILED"},
		"not found":       {mikrotik.ErrRouterNotFound, 404, "MIKROTIK_ROUTER_NOT_FOUND"},
		"state not saved": {mikrotik.ErrSetupStateNotSaved, 500, "MIKROTIK_SETUP_STATE_NOT_SAVED"},
		"unknown":         {errors.New("boom password=" + testPassword), 500, "INTERNAL_ERROR"},
		"in progress":     {mikrotik.ErrSetupInProgress, 409, "MIKROTIK_SETUP_IN_PROGRESS"},
		"mark failed":     {mikrotik.ErrSetupMarkFailed, 500, "MIKROTIK_SETUP_MARK_FAILED"},
		"rest unverified": {mikrotik.ErrSetupRESTUnverified, 422, "MIKROTIK_SETUP_REST_UNVERIFIED"},
	} {
		t.Run(name, func(t *testing.T) {
			r := setupRouterFor(t, &fakeSetupService{
				cleanupFn: func() (domain.Router, error) { return domain.Router{}, tc.err },
			})
			w := performJSON(r, http.MethodPost, setupBase+"/cleanup", "")
			if code, _ := errorBody(t, w.Body.Bytes()); w.Code != tc.status || code != tc.code {
				t.Fatalf("status = %d code = %s, body = %s", w.Code, code, w.Body)
			}
			if strings.Contains(w.Body.String(), testPassword) {
				t.Fatal("response leaks password")
			}
		})
	}
}

// storeRepo is the smallest repository the real SetupService needs.
type storeRepo struct {
	repository.RouterRepository
	router domain.Router
}

func (s *storeRepo) FindByID(context.Context, coretenant.Scope, string) (domain.Router, error) {
	return s.router, nil
}
func (s *storeRepo) MarkSetup(context.Context, coretenant.Scope, string, bool, string, string) error {
	return nil
}
func (s *storeRepo) InsertAudit(context.Context, coretenant.Scope, repository.AuditEntry) error {
	return nil
}

// noopLocker always grants the setup lock (the real lock is tested in the
// repository and service packages).
type noopLocker struct{}

func (noopLocker) TryLock(context.Context, string) (func(), bool, error) { return func() {}, true, nil }

type fixedFactory struct{ client pmk.RouterClient }

func (f fixedFactory) Detect(context.Context, pmk.Endpoint) (string, string, error) {
	return "binary", "7.8", nil
}
func (f fixedFactory) New(string, pmk.Endpoint) (pmk.RouterClient, error) { return f.client, nil }

func realSetupEngine(t *testing.T, client *pmk.FakeClient) *gin.Engine {
	t.Helper()
	const secret = "unit-test-app-secret-32-bytes-long!!"
	enc, err := corecrypto.EncryptSecret(secret, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	repo := &storeRepo{router: domain.Router{
		ID: setupRouterID, Name: "lab", Host: "10.8.0.2", Port: 8728, APIType: domain.APITypeBinary,
		Username: "api", PasswordEncrypted: enc,
	}}
	rs := service.NewRouterService(repo, fixedFactory{client}, secret, []netip.Prefix{netip.MustParsePrefix("10.8.0.0/24")})
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "user-1") })
	NewSetupHandler(service.NewSetupService(rs, repo, noopLocker{}, service.SetupOptions{})).RegisterRoutes(r.Group("/app/mikrotik"), nil, nil)
	return r
}

func TestPreviewDoesNotWriteToRouter(t *testing.T) {
	client := pmk.NewFakeClient()
	r := realSetupEngine(t, client)
	w := performJSON(r, http.MethodPost, setupBase+"/preview", setupBody)
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
	if client.WriteCalls() != 0 {
		t.Fatalf("preview wrote %d times", client.WriteCalls())
	}
	for _, want := range []string{`"hash":"`, `"kind":"create"`, `"comment":"zyad-managed"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("body missing %s: %s", want, w.Body)
		}
	}
	if strings.Contains(w.Body.String(), testPassword) {
		t.Fatal("leaks password")
	}
}

func TestPreviewThenApplyOverHTTP(t *testing.T) {
	client := pmk.NewFakeClient()
	r := realSetupEngine(t, client)
	w := performJSON(r, http.MethodPost, setupBase+"/preview", setupBody)
	hash := between(w.Body.String(), `"hash":"`, `"`)
	if len(hash) != 64 {
		t.Fatalf("hash = %q (%s)", hash, w.Body)
	}
	body := strings.TrimSuffix(setupBody, "}") + `,"plan_hash":"` + hash + `"}`
	if w := performJSON(r, http.MethodPost, setupBase+"/apply", body); w.Code != 200 {
		t.Fatalf("apply status = %d, body = %s", w.Code, w.Body)
	}
	if client.WriteCalls() != 9 {
		t.Fatalf("writes = %d, want 9", client.WriteCalls())
	}
	// Same hash again: the router has changed, so the stored plan is stale.
	w = performJSON(r, http.MethodPost, setupBase+"/apply", body)
	if code, _ := errorBody(t, w.Body.Bytes()); w.Code != 409 || code != "MIKROTIK_SETUP_PLAN_CHANGED" {
		t.Fatalf("replay status = %d, body = %s", w.Code, w.Body)
	}
	if client.WriteCalls() != 9 {
		t.Fatal("stale replay wrote")
	}
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	s = s[i+len(start):]
	j := strings.Index(s, end)
	if j < 0 {
		return ""
	}
	return s[:j]
}

func TestSetupRoutesRequireSetupPermission(t *testing.T) {
	routes := []struct{ method, path string }{
		{http.MethodPost, "/app/mikrotik/routers/:id/setup/preview"},
		{http.MethodPost, "/app/mikrotik/routers/:id/setup/apply"},
		{http.MethodPost, "/app/mikrotik/routers/:id/setup/cleanup"},
	}
	engine := newTestRouter(t)
	NewSetupHandler(&fakeSetupService{}).RegisterRoutes(engine.Group("/app/mikrotik"), nil, nil)
	registered := map[string]bool{}
	for _, ri := range engine.Routes() {
		registered[ri.Method+" "+ri.Path] = true
	}
	if len(registered) != len(routes) {
		t.Fatalf("registered %v, want %d routes", registered, len(routes))
	}
	for _, rt := range routes {
		if !registered[rt.method+" "+rt.path] {
			t.Errorf("route %s %s not registered", rt.method, rt.path)
		}
		checker := &recordingChecker{}
		r := newTestRouter(t)
		r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "user-1") })
		NewSetupHandler(&fakeSetupService{}).RegisterRoutes(r.Group("/app/mikrotik"), checker, nil)
		w := performJSON(r, rt.method, strings.ReplaceAll(rt.path, ":id", setupRouterID), `{}`)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", rt.path, w.Code)
		}
		if len(checker.asked) == 0 || checker.asked[0] != "mikrotik_router.setup" {
			t.Errorf("%s: asked %v, want mikrotik_router.setup", rt.path, checker.asked)
		}
	}
}

func TestSetupFailedMessageIsOperationSpecific(t *testing.T) {
	for _, tc := range []struct {
		step, must, mustNot string
	}{
		{"create /ip/firewall/filter", "apply is safe to retry", "cleanup"},
		{"remove /ip/pool", "cleanup is incomplete and can be retried", "apply is safe"},
	} {
		r := setupRouterFor(t, &fakeSetupService{cleanupFn: func() (domain.Router, error) {
			return domain.Router{}, &mikrotik.SetupStepError{Step: tc.step, Cause: mikrotik.ErrSetupFailed}
		}})
		w := performJSON(r, http.MethodPost, setupBase+"/cleanup", "")
		if w.Code != 502 || !strings.Contains(w.Body.String(), tc.must) || strings.Contains(w.Body.String(), tc.mustNot) {
			t.Errorf("%s: status = %d body = %s", tc.step, w.Code, w.Body)
		}
	}
	r := setupRouterFor(t, &fakeSetupService{cleanupFn: func() (domain.Router, error) {
		return domain.Router{}, mikrotik.ErrSetupMarkFailed
	}})
	if w := performJSON(r, http.MethodPost, setupBase+"/cleanup", ""); w.Code != 500 || !strings.Contains(w.Body.String(), "nothing was changed") {
		t.Errorf("mark failed: %d %s", w.Code, w.Body)
	}
}
