package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/repository"
	"zyad.cloud/internal/modules/mikrotik/service"
)

const (
	testSecretID  = "44444444-4444-4444-4444-444444444444"
	testAccountID = "55555555-5555-5555-5555-555555555555"
	testRouterID  = "33333333-3333-3333-3333-333333333333"
)

type fakeSecretService struct {
	err error

	importRouter string
	listFilter   repository.SecretFilter
	listLimit    int
	listOffset   int
	pairedWith   string
	profile      string
	searchQ      string
	searchLimit  int
	calls        int

	analyzeRouter string
	analyzeRes    service.AnalyzeResult
	bulkItems     []service.PairItem
	bulkResults   []service.PairResult
}

func (f *fakeSecretService) Analyze(_ context.Context, _ coretenant.Scope, routerID, _ string) (service.AnalyzeResult, error) {
	f.calls++
	f.analyzeRouter = routerID
	return f.analyzeRes, f.err
}
func (f *fakeSecretService) PairBulk(_ context.Context, _ coretenant.Scope, items []service.PairItem, _ string) []service.PairResult {
	f.calls++
	f.bulkItems = items
	return f.bulkResults
}

func (f *fakeSecretService) Import(_ context.Context, _ coretenant.Scope, routerID, _ string) (repository.ImportResult, error) {
	f.calls++
	f.importRouter = routerID
	return repository.ImportResult{Created: 2, Updated: 1, Missing: 3}, f.err
}
func (f *fakeSecretService) List(_ context.Context, _ coretenant.Scope, filter repository.SecretFilter, limit, offset int) ([]domain.PPPSecret, int64, error) {
	f.calls++
	f.listFilter, f.listLimit, f.listOffset = filter, limit, offset
	return []domain.PPPSecret{sampleSecret()}, 1, f.err
}
func (f *fakeSecretService) Pair(_ context.Context, _ coretenant.Scope, _, accountID, _ string) (domain.PPPSecret, error) {
	f.calls++
	f.pairedWith = accountID
	return sampleSecret(), f.err
}
func (f *fakeSecretService) PairNewCustomer(context.Context, coretenant.Scope, string, string) (domain.PPPSecret, error) {
	f.calls++
	return sampleSecret(), f.err
}
func (f *fakeSecretService) Unpair(context.Context, coretenant.Scope, string, string) error {
	f.calls++
	return f.err
}
func (f *fakeSecretService) SetOriginalProfile(_ context.Context, _ coretenant.Scope, _, profile, _ string) error {
	f.calls++
	f.profile = profile
	return f.err
}
func (f *fakeSecretService) SearchAccounts(_ context.Context, _ coretenant.Scope, q string, limit int) ([]service.AccountRef, error) {
	f.calls++
	f.searchQ, f.searchLimit = q, limit
	return []service.AccountRef{{ID: testAccountID, Name: "Alice", Phone: "0812", Email: "a@example.com"}}, f.err
}

func sampleSecret() domain.PPPSecret {
	acc, now := testAccountID, time.Now()
	return domain.PPPSecret{
		ID: testSecretID, RouterID: testRouterID, Name: "alice", Profile: "paket-10m", OriginalProfile: "paket-10m",
		Present: true, IsolationState: domain.IsolationNormal, AccountID: &acc, PairedAt: &now, LastSeenAt: &now,
		Comment: "rumah", CreatedAt: now, UpdatedAt: now,
	}
}

func secretRouter(t *testing.T, svc SecretService) *gin.Engine {
	r := newTestRouter(t)
	NewSecretHandler(svc).RegisterRoutes(r.Group("/app/mikrotik"), nil, nil)
	return r
}

func TestSecretRoutes_Success(t *testing.T) {
	const base = "/app/mikrotik"
	for _, tc := range []struct {
		name, method, path, body string
		want                     int
	}{
		{"import", http.MethodPost, base + "/routers/" + testRouterID + "/secrets/import", ``, http.StatusOK},
		{"list", http.MethodGet, base + "/secrets", ``, http.StatusOK},
		{"pair", http.MethodPost, base + "/secrets/" + testSecretID + "/pair", `{"account_id":"` + testAccountID + `"}`, http.StatusOK},
		{"pair new customer", http.MethodPost, base + "/secrets/" + testSecretID + "/pair-new-customer", ``, http.StatusOK},
		{"unpair", http.MethodPost, base + "/secrets/" + testSecretID + "/unpair", ``, http.StatusNoContent},
		{"original profile", http.MethodPut, base + "/secrets/" + testSecretID + "/original-profile", `{"profile":"paket-10m"}`, http.StatusNoContent},
		{"account search", http.MethodGet, base + "/accounts/search?q=ali", ``, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := performJSON(secretRouter(t, &fakeSecretService{}), tc.method, tc.path, tc.body)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d, body = %s", w.Code, tc.want, w.Body)
			}
			assertNoCredentialKeys(t, w.Body.Bytes())
		})
	}
}

func TestSecretRoutes_ErrorMapping(t *testing.T) {
	const base = "/app/mikrotik"
	wrapped := fmt.Errorf("%w: secret is no longer on the router", mikrotik.ErrSecretNotPairable)
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"invalid input", &mikrotik.InvalidInputError{Message: "profile does not exist on the router"}, http.StatusUnprocessableEntity},
		{"router not found", mikrotik.ErrRouterNotFound, http.StatusNotFound},
		{"secret not found", mikrotik.ErrSecretNotFound, http.StatusNotFound},
		{"account not found", mikrotik.ErrAccountNotFound, http.StatusNotFound},
		{"already paired", mikrotik.ErrSecretAlreadyPaired, http.StatusConflict},
		{"isolated", mikrotik.ErrSecretIsolated, http.StatusConflict},
		{"active task", mikrotik.ErrSecretHasActiveTask, http.StatusConflict},
		{"not pairable", wrapped, http.StatusConflict},
		{"unreachable", mikrotik.ErrUnreachable, http.StatusBadGateway},
		{"auth failed", mikrotik.ErrAuthFailed, http.StatusBadGateway},
		{"unknown hides detail", errors.New("dial 10.0.0.1 password=hunter2"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := secretRouter(t, &fakeSecretService{err: tc.err})
			for _, rt := range []struct{ method, path, body string }{
				{http.MethodPost, base + "/routers/" + testRouterID + "/secrets/import", ``},
				{http.MethodPost, base + "/secrets/" + testSecretID + "/pair", `{"account_id":"` + testAccountID + `"}`},
				{http.MethodPost, base + "/secrets/" + testSecretID + "/unpair", ``},
				{http.MethodPut, base + "/secrets/" + testSecretID + "/original-profile", `{"profile":"p"}`},
			} {
				w := performJSON(r, rt.method, rt.path, rt.body)
				if w.Code != tc.want {
					t.Errorf("%s %s: status = %d, want %d, body = %s", rt.method, rt.path, w.Code, tc.want, w.Body)
				}
				if strings.Contains(w.Body.String(), "hunter2") {
					t.Errorf("unknown error detail leaked: %s", w.Body)
				}
			}
		})
	}
}

func TestSecretList_PerPageClampAndFilter(t *testing.T) {
	svc := &fakeSecretService{}
	r := secretRouter(t, svc)
	w := performJSON(r, http.MethodGet, "/app/mikrotik/secrets?router_id="+testRouterID+"&paired=false&q=ali&page=3&per_page=9999", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body)
	}
	if svc.listLimit != domain.MaxRouterPageSize || svc.listOffset != 2*domain.MaxRouterPageSize {
		t.Fatalf("limit/offset = %d/%d", svc.listLimit, svc.listOffset)
	}
	if svc.listFilter.RouterID != testRouterID || svc.listFilter.Search != "ali" || svc.listFilter.Paired == nil || *svc.listFilter.Paired {
		t.Fatalf("filter = %+v", svc.listFilter)
	}
	var body struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Page    int `json:"page"`
			PerPage int `json:"per_page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Meta.PerPage != domain.MaxRouterPageSize || body.Meta.Page != 3 {
		t.Fatalf("meta = %+v", body.Meta)
	}
	if len(body.Data) != 1 {
		t.Fatalf("data = %v", body.Data)
	}
	for _, key := range []string{"id", "router_id", "name", "profile", "original_profile", "needs_original_profile",
		"disabled", "present", "isolation_state", "account_id", "paired_at", "last_seen_at", "comment"} {
		if _, ok := body.Data[0][key]; !ok {
			t.Errorf("secret response lacks %q", key)
		}
	}
}

func TestSecretList_RejectsBadQuery(t *testing.T) {
	for _, q := range []string{"?router_id=not-a-uuid", "?paired=maybe"} {
		w := performJSON(secretRouter(t, &fakeSecretService{}), http.MethodGet, "/app/mikrotik/secrets"+q, "")
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d", q, w.Code)
		}
	}
}

func TestSecretBodies_RejectUnknownAndInvalid(t *testing.T) {
	const base = "/app/mikrotik/secrets/" + testSecretID
	for _, tc := range []struct{ name, method, path, body string }{
		{"pair unknown field", http.MethodPost, base + "/pair", `{"account_id":"` + testAccountID + `","password":"x"}`},
		{"pair missing account", http.MethodPost, base + "/pair", `{}`},
		{"pair empty body", http.MethodPost, base + "/pair", ``},
		{"pair bad uuid", http.MethodPost, base + "/pair", `{"account_id":"nope"}`},
		{"pair trailing data", http.MethodPost, base + "/pair", `{"account_id":"` + testAccountID + `"}{}`},
		{"profile unknown field", http.MethodPut, base + "/original-profile", `{"profile":"p","x":1}`},
		{"profile missing", http.MethodPut, base + "/original-profile", `{}`},
		{"profile too long", http.MethodPut, base + "/original-profile", `{"profile":"` + strings.Repeat("x", 101) + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeSecretService{}
			w := performJSON(secretRouter(t, svc), tc.method, tc.path, tc.body)
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s", w.Code, w.Body)
			}
			if svc.calls != 0 {
				t.Fatal("service must not be called")
			}
		})
	}
}

func TestSecretRoutes_MalformedIDIs404(t *testing.T) {
	svc := &fakeSecretService{}
	r := secretRouter(t, svc)
	for _, rt := range []struct{ method, path, body string }{
		{http.MethodPost, "/app/mikrotik/routers/x/secrets/import", ``},
		{http.MethodPost, "/app/mikrotik/secrets/x/pair", `{"account_id":"` + testAccountID + `"}`},
		{http.MethodPost, "/app/mikrotik/secrets/x/pair-new-customer", ``},
		{http.MethodPost, "/app/mikrotik/secrets/x/unpair", ``},
		{http.MethodPut, "/app/mikrotik/secrets/x/original-profile", `{"profile":"p"}`},
	} {
		if w := performJSON(r, rt.method, rt.path, rt.body); w.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d", rt.method, rt.path, w.Code)
		}
	}
	if svc.calls != 0 {
		t.Fatal("service must not be called")
	}
}

func TestSecretPassesArguments(t *testing.T) {
	svc := &fakeSecretService{}
	r := secretRouter(t, svc)
	performJSON(r, http.MethodPost, "/app/mikrotik/secrets/"+testSecretID+"/pair", `{"account_id":"`+testAccountID+`"}`)
	performJSON(r, http.MethodPut, "/app/mikrotik/secrets/"+testSecretID+"/original-profile", `{"profile":"paket-20m"}`)
	performJSON(r, http.MethodPost, "/app/mikrotik/routers/"+testRouterID+"/secrets/import", ``)
	performJSON(r, http.MethodGet, "/app/mikrotik/accounts/search?q=ali", ``)
	if svc.pairedWith != testAccountID || svc.profile != "paket-20m" || svc.importRouter != testRouterID || svc.searchQ != "ali" || svc.searchLimit != accountSearchLimit {
		t.Fatalf("args = %+v", svc)
	}
}

func TestAccountSearch_RequiresQuery(t *testing.T) {
	w := performJSON(secretRouter(t, &fakeSecretService{}), http.MethodGet, "/app/mikrotik/accounts/search", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestSecretRoutesRequirePermissions(t *testing.T) {
	routes := []struct{ method, path, perm string }{
		{http.MethodPost, "/app/mikrotik/routers/:id/secrets/import", "mikrotik_secret.import"},
		{http.MethodGet, "/app/mikrotik/secrets", "mikrotik_secret.read"},
		{http.MethodPost, "/app/mikrotik/secrets/:id/pair", "mikrotik_secret.pair"},
		{http.MethodPost, "/app/mikrotik/secrets/:id/pair-new-customer", "mikrotik_secret.pair"},
		{http.MethodPost, "/app/mikrotik/secrets/:id/unpair", "mikrotik_secret.unpair"},
		{http.MethodPut, "/app/mikrotik/secrets/:id/original-profile", "mikrotik_secret.pair"},
		{http.MethodGet, "/app/mikrotik/accounts/search", "mikrotik_secret.read"},
		{http.MethodPost, "/app/mikrotik/routers/:id/secrets/analyze", "mikrotik_secret.import"},
		{http.MethodPost, "/app/mikrotik/secrets/pair-bulk", "mikrotik_secret.pair"},
	}
	assertRoutesRequirePermissions(t, routes, func(g *gin.RouterGroup, p *recordingChecker) {
		NewSecretHandler(&fakeSecretService{}).RegisterRoutes(g, p, nil)
	})
}

func TestResponseDTOsNeverCarryCredentials(t *testing.T) {
	now := time.Now()
	for name, v := range map[string]any{
		"secret":      dto.SecretFromDomain(sampleSecret()),
		"secret list": dto.SecretListFromDomain([]domain.PPPSecret{sampleSecret()}),
		"import":      dto.ImportResultFromRepo(repository.ImportResult{Created: 1}),
		"accounts":    dto.AccountListFromService([]service.AccountRef{{ID: "a", Name: "n"}}),
		"policy":      dto.PolicyFromDomain(domain.CustomerPolicy{AccountID: "a", SetAt: now}),
		"product":     dto.ProductListFromService([]service.ProductInfo{{ID: "p"}}),
		"trigger":     dto.TriggerFromDomain(domain.ProductTrigger{ProductID: "p", EffectiveFrom: now}),
		"preview":     dto.TriggerPreviewFromService(service.TriggerPreview{EffectiveFrom: now}),
	} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		assertNoCredentialKeys(t, b)
	}
}

// assertNoCredentialKeys fails when any JSON key (at any depth) looks like a credential.
func assertNoCredentialKeys(t *testing.T, raw []byte) {
	t.Helper()
	if len(raw) == 0 {
		return
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	var walk func(any)
	walk = func(n any) {
		switch x := n.(type) {
		case map[string]any:
			for k, child := range x {
				lk := strings.ToLower(k)
				if strings.Contains(lk, "password") || strings.Contains(lk, "credential") || lk == "secret" {
					t.Errorf("response carries credential-like key %q", k)
				}
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(v)
}

// assertRoutesRequirePermissions registers a handler with a denying checker and
// checks every route is registered, consults the checker with the expected
// permission first, and is rejected with 403.
func assertRoutesRequirePermissions(t *testing.T, routes []struct{ method, path, perm string }, register func(*gin.RouterGroup, *recordingChecker)) {
	t.Helper()
	engine := newTestRouter(t)
	register(engine.Group("/app/mikrotik"), &recordingChecker{})
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
		checker := &recordingChecker{}
		r := newTestRouter(t)
		r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "user-1") })
		register(r.Group("/app/mikrotik"), checker)
		path := strings.NewReplacer(":id", testRouterID, ":account_id", testAccountID).Replace(rt.path)
		w := performJSON(r, rt.method, path, `{}`)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", rt.method, rt.path, w.Code)
		}
		if len(checker.asked) == 0 || checker.asked[0] != rt.perm {
			t.Errorf("%s %s: asked %v, want %s", rt.method, rt.path, checker.asked, rt.perm)
		}
	}
}

func TestAnalyzeRoute_RequiresImportPermission(t *testing.T) {
	checker := &recordingChecker{}
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "user-1") })
	svc := &fakeSecretService{}
	NewSecretHandler(svc).RegisterRoutes(r.Group("/app/mikrotik"), checker, nil)
	w := performJSON(r, http.MethodPost, "/app/mikrotik/routers/"+testRouterID+"/secrets/analyze", ``)
	if w.Code != http.StatusForbidden || svc.calls != 0 {
		t.Fatalf("status = %d calls = %d", w.Code, svc.calls)
	}
	if len(checker.asked) == 0 || checker.asked[0] != "mikrotik_secret.import" {
		t.Fatalf("asked %v", checker.asked)
	}
}

func TestAnalyzeRoute_ReturnsCountsAndMapsInProgress(t *testing.T) {
	url := "/app/mikrotik/routers/" + testRouterID + "/secrets/analyze"
	svc := &fakeSecretService{analyzeRes: service.AnalyzeResult{Total: 5, Suggested: 3, NeedsDecision: 2}}
	w := performJSON(secretRouter(t, svc), http.MethodPost, url, ``)
	if w.Code != http.StatusOK || svc.analyzeRouter != testRouterID {
		t.Fatalf("status=%d router=%q body=%s", w.Code, svc.analyzeRouter, w.Body)
	}
	var body struct{ Data map[string]int }
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data["total"] != 5 || body.Data["suggested"] != 3 || body.Data["needs_decision"] != 2 {
		t.Fatalf("data = %v", body.Data)
	}
	w = performJSON(secretRouter(t, &fakeSecretService{err: mikrotik.ErrAnalyzeInProgress}), http.MethodPost, url, ``)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "MIKROTIK_ANALYZE_IN_PROGRESS") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if w := performJSON(secretRouter(t, &fakeSecretService{}), http.MethodPost, "/app/mikrotik/routers/x/secrets/analyze", ``); w.Code != http.StatusNotFound {
		t.Fatalf("malformed id status = %d", w.Code)
	}
}

func TestListSecrets_SuggestionFilter(t *testing.T) {
	svc := &fakeSecretService{}
	r := secretRouter(t, svc)
	for _, v := range []string{"suggested", "needs_decision"} {
		w := performJSON(r, http.MethodGet, "/app/mikrotik/secrets?paired=false&suggestion="+v, "")
		if w.Code != http.StatusOK || svc.listFilter.Suggestion != v || svc.listFilter.Paired == nil || *svc.listFilter.Paired {
			t.Fatalf("%s: status=%d filter=%+v", v, w.Code, svc.listFilter)
		}
	}
	calls := svc.calls
	if w := performJSON(r, http.MethodGet, "/app/mikrotik/secrets?suggestion=bogus", ""); w.Code != http.StatusUnprocessableEntity || svc.calls != calls {
		t.Fatalf("bogus: status=%d", w.Code)
	}
}

func TestListSecrets_ResponseCarriesSuggestion(t *testing.T) {
	sec := sampleSecret()
	sec.AccountID = nil
	b, _ := json.Marshal(dto.SecretListFromDomain([]domain.PPPSecret{
		sec,
		func() domain.PPPSecret {
			x := sec
			x.Suggestion = &domain.SecretSuggestion{AccountID: testAccountID, AccountName: "Alice", AccountPhone: "0812", Basis: "phone", Candidates: 1}
			return x
		}(),
	}))
	var out []map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if v, ok := out[0]["suggestion"]; !ok || v != nil {
		t.Fatalf("want explicit null suggestion, got %v (present=%v)", v, ok)
	}
	sg, _ := out[1]["suggestion"].(map[string]any)
	if sg["account_id"] != testAccountID || sg["account_name"] != "Alice" || sg["account_phone"] != "0812" || sg["basis"] != "phone" || sg["candidates"].(float64) != 1 {
		t.Fatalf("suggestion = %v", sg)
	}
}

const bulkURL = "/app/mikrotik/secrets/pair-bulk"

func TestPairBulk_PartialFailureReportsPerRow(t *testing.T) {
	const second = "66666666-6666-6666-6666-666666666666"
	svc := &fakeSecretService{bulkResults: []service.PairResult{
		{SecretID: testSecretID, OK: false, Code: "MIKROTIK_SECRET_ALREADY_PAIRED", Message: "ppp secret is already paired to a customer"},
		{SecretID: second, OK: true},
	}}
	body := `{"items":[{"secret_id":"` + testSecretID + `","account_id":"` + testAccountID + `"},{"secret_id":"` + second + `","account_id":"` + testAccountID + `"}]}`
	w := performJSON(secretRouter(t, svc), http.MethodPost, bulkURL, body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body)
	}
	var out struct {
		Data struct {
			Results []struct {
				SecretID string `json:"secret_id"`
				OK       bool   `json:"ok"`
				Code     string `json:"code"`
				Message  string `json:"message"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	r := out.Data.Results
	if len(r) != 2 || r[0].SecretID != testSecretID || r[0].OK || r[0].Code != "MIKROTIK_SECRET_ALREADY_PAIRED" || !r[1].OK || r[1].SecretID != second {
		t.Fatalf("results = %+v", r)
	}
	if len(svc.bulkItems) != 2 || svc.bulkItems[0].AccountID != testAccountID {
		t.Fatalf("items = %+v", svc.bulkItems)
	}
}

func TestPairBulk_RejectsDuplicateSecretID(t *testing.T) {
	svc := &fakeSecretService{}
	item := `{"secret_id":"` + testSecretID + `","account_id":"` + testAccountID + `"}`
	w := performJSON(secretRouter(t, svc), http.MethodPost, bulkURL, `{"items":[`+item+`,`+item+`]}`)
	if w.Code != http.StatusUnprocessableEntity || svc.calls != 0 {
		t.Fatalf("status = %d calls = %d body = %s", w.Code, svc.calls, w.Body)
	}
}

func TestPairBulk_RejectsEmptyAndOver200(t *testing.T) {
	var many []string
	for i := 0; i < 201; i++ {
		many = append(many, fmt.Sprintf(`{"secret_id":"00000000-0000-4000-8000-%012d","account_id":"%s"}`, i, testAccountID))
	}
	for name, body := range map[string]string{
		"empty items":  `{"items":[]}`,
		"missing":      `{}`,
		"over 200":     `{"items":[` + strings.Join(many, ",") + `]}`,
		"bad uuid":     `{"items":[{"secret_id":"x","account_id":"` + testAccountID + `"}]}`,
		"unknown key":  `{"items":[{"secret_id":"` + testSecretID + `","account_id":"` + testAccountID + `","x":1}]}`,
		"missing acct": `{"items":[{"secret_id":"` + testSecretID + `"}]}`,
	} {
		svc := &fakeSecretService{}
		w := performJSON(secretRouter(t, svc), http.MethodPost, bulkURL, body)
		if w.Code != http.StatusUnprocessableEntity || svc.calls != 0 {
			t.Errorf("%s: status = %d calls = %d", name, w.Code, svc.calls)
		}
	}
	// Exactly 200 is accepted.
	svc := &fakeSecretService{}
	w := performJSON(secretRouter(t, svc), http.MethodPost, bulkURL, `{"items":[`+strings.Join(many[:200], ",")+`]}`)
	if w.Code != http.StatusOK || len(svc.bulkItems) != 200 {
		t.Fatalf("200 items: status = %d", w.Code)
	}
}
