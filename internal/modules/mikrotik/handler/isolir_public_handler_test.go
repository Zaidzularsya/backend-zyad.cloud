package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"

	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/service"
)

type fakePublicSvc struct {
	mu      sync.Mutex
	info    service.PublicInfo
	result  service.PublicLookupResult
	err     error
	queries []string
	today   time.Time
}

func (f *fakePublicSvc) Info(context.Context, coretenant.Scope) (service.PublicInfo, error) {
	return f.info, f.err
}

func (f *fakePublicSvc) Lookup(_ context.Context, _ coretenant.Scope, q string, today time.Time) (service.PublicLookupResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, q)
	f.today = today
	return f.result, f.err
}

type isolirCounter struct {
	mu     sync.Mutex
	counts map[string]int64
}

func (f *isolirCounter) Incr(ctx context.Context, key string) *goredis.IntCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.counts == nil {
		f.counts = map[string]int64{}
	}
	f.counts[key]++
	cmd := goredis.NewIntCmd(ctx)
	cmd.SetVal(f.counts[key])
	return cmd
}

// sumPrefix totals the counters whose key starts with prefix (keys carry a window suffix).
func (f *isolirCounter) sumPrefix(prefix string) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for k, v := range f.counts {
		if strings.HasPrefix(k, prefix) {
			n += v
		}
	}
	return n
}

func (f *isolirCounter) Expire(ctx context.Context, _ string, _ time.Duration) *goredis.BoolCmd {
	return goredis.NewBoolCmd(ctx)
}

type isolirHostResolver struct{ t *testing.T }

func (h isolirHostResolver) ResolvePublicHost(_ context.Context, host string) (coretenant.Context, bool, error) {
	if host != "isp.example.com" {
		return coretenant.Context{}, false, nil
	}
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "11111111-1111-4111-8111-111111111111", OrganizationSlug: "isp",
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", MembershipStatus: "active", MembershipVersion: 1,
		ResolutionSource: coretenant.ResolutionSourceCustomDomain, DataPlacement: coretenant.DataPlacementShared, RequestHost: host,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return tc, true, nil
}

func isolirRouter(t *testing.T, svc PublicService, counter middleware.RateCounter) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mw, err := middleware.ResolvePublicOrganization(isolirHostResolver{t}, middleware.PublicHostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	h := NewIsolirPublicHandler(svc, counter)
	h.sleep = func(context.Context, time.Duration) {} // the response floor has its own tests
	h.RegisterRoutes(r.Group("", mw, middleware.RequireActiveTenant()))
	return r
}

func isolirReq(r http.Handler, method, path, ip, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = "isp.example.com"
	req.RemoteAddr = ip + ":5555"
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func lookup(r http.Handler, ip, body string) *httptest.ResponseRecorder {
	return isolirReq(r, http.MethodPost, "/public/isolir/lookup", ip, body, nil)
}

const uniformBody = `{"success":true,"message":"ok","data":{"found":false,"masked_name":"","isolated":false,"invoices":[]}}`

func TestLookup_BodyShapeUniformAcrossCases(t *testing.T) {
	// unmatched, no debt, ambiguous and feature-disabled all reach the handler as the zero result.
	r := isolirRouter(t, &fakePublicSvc{}, nil)
	var bodies []string
	for _, q := range []string{"budi", "08123456789", "ambiguous-user", "disabled"} {
		w := lookup(r, "203.0.113.5", `{"query":"`+q+`"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: code=%d", q, w.Code)
		}
		bodies = append(bodies, strings.TrimSpace(w.Body.String()))
	}
	for _, b := range bodies {
		if b != uniformBody {
			t.Fatalf("body=%s want %s", b, uniformBody)
		}
	}
}

func TestLookup_FoundBodyShape(t *testing.T) {
	svc := &fakePublicSvc{result: service.PublicLookupResult{
		Found: true, MaskedName: "B*** S******", Isolated: true,
		Invoices: []service.PublicInvoiceItem{{Number: "INV-1", DueDate: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), Balance: "150000.00", PayURL: "https://pay/x"}},
	}}
	w := lookup(isolirRouter(t, svc, nil), "203.0.113.5", `{"query":"budi"}`)
	want := `{"success":true,"message":"ok","data":{"found":true,"masked_name":"B*** S******","isolated":true,"invoices":[{"number":"INV-1","due_date":"2026-11-01","balance":"150000.00","pay_url":"https://pay/x"}]}}`
	if strings.TrimSpace(w.Body.String()) != want {
		t.Fatalf("body=%s", w.Body.String())
	}
	if svc.today.IsZero() || svc.today.Hour() != 0 || svc.today.Location() != time.UTC {
		t.Fatalf("today must be a business day at UTC midnight: %v", svc.today)
	}
}

func TestLookup_EmptyOrOversizedQuery_Uniform(t *testing.T) {
	r := isolirRouter(t, &fakePublicSvc{}, nil)
	bodies := []string{
		`{"query":""}`,
		`{"query":"` + strings.Repeat("a", 101) + `"}`,
		`{"query":"a\u0000b"}`,
		`{"query":"` + strings.Repeat("a", 5000) + `"}`, // over the body cap
		`{not json`,
		``,
		`{}`,
		`{"query":123}`,
	}
	for i, b := range bodies {
		w := lookup(r, "203.0.113.5", b)
		if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != uniformBody {
			t.Fatalf("case %d: code=%d body=%s", i, w.Code, w.Body.String())
		}
	}
}

func TestLookup_HeadersNoStoreNoindex(t *testing.T) {
	r := isolirRouter(t, &fakePublicSvc{}, nil)
	for _, w := range []*httptest.ResponseRecorder{
		lookup(r, "203.0.113.5", `{"query":"x"}`),
		isolirReq(r, http.MethodGet, "/public/isolir/info", "203.0.113.5", "", nil),
		lookup(isolirRouter(t, &fakePublicSvc{err: errors.New("boom")}, nil), "203.0.113.5", `{"query":"x"}`),
	} {
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Robots-Tag") != "noindex" {
			t.Fatalf("code=%d headers=%v", w.Code, w.Header())
		}
	}
}

func TestLookup_RejectsOrganizationHeaderSelector(t *testing.T) {
	r := isolirRouter(t, &fakePublicSvc{}, nil)
	w := isolirReq(r, http.MethodPost, "/public/isolir/lookup", "203.0.113.5", `{"query":"x"}`,
		map[string]string{middleware.OrganizationIDHeader: "22222222-2222-4222-8222-222222222222"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestLookup_NoSensitiveFieldsInJSON(t *testing.T) {
	svc := &fakePublicSvc{result: service.PublicLookupResult{
		Found: true, MaskedName: "B***",
		Invoices: []service.PublicInvoiceItem{{Number: "INV-1", DueDate: time.Now(), Balance: "1.00"}},
	}}
	w := lookup(isolirRouter(t, svc, nil), "203.0.113.5", `{"query":"x"}`)
	for _, bad := range []string{"phone", "email", "router", "secret", "profile", "host"} {
		if strings.Contains(strings.ToLower(w.Body.String()), bad) {
			t.Fatalf("body leaks %q: %s", bad, w.Body.String())
		}
	}
}

func TestLookup_RateLimitPerIPAndQuery(t *testing.T) {
	counter := &isolirCounter{}
	svc := &fakePublicSvc{}
	r := isolirRouter(t, svc, counter)
	// same query from different IPs: the 6th hits the per-query bucket.
	var last *httptest.ResponseRecorder
	for i := 0; i < 6; i++ {
		last = lookup(r, "203.0.113."+string(rune('1'+i)), `{"query":"Budi"}`)
	}
	if last.Code != http.StatusTooManyRequests || !strings.Contains(last.Body.String(), "RATE_LIMITED") {
		t.Fatalf("code=%d body=%s", last.Code, last.Body.String())
	}
	// normalization: different case/spacing of the same query shares the bucket.
	if w := lookup(r, "198.51.100.1", `{"query":"  budi "}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("normalized query must share bucket: %d", w.Code)
	}
	// 429 body is the same for an unknown query that exhausted its own bucket.
	var other *httptest.ResponseRecorder
	for i := 0; i < 6; i++ {
		other = lookup(r, "198.51.100.2", `{"query":"nobody"}`)
	}
	if other.Code != http.StatusTooManyRequests || other.Body.String() != last.Body.String() {
		t.Fatalf("429 bodies differ: %s vs %s", other.Body.String(), last.Body.String())
	}
	// raw query never appears in a Redis key.
	for k := range counter.counts {
		if strings.Contains(strings.ToLower(k), "budi") || strings.Contains(k, "nobody") {
			t.Fatalf("raw query in key %q", k)
		}
	}
}

// Isolated customers of one ISP share a NAT IP: the per-IP lookup bucket
// allows 120/min and info has its own 300/min bucket.
func TestLookup_PerIPLimitFitsSharedNAT(t *testing.T) {
	counter := &isolirCounter{}
	r := isolirRouter(t, &fakePublicSvc{}, counter)
	for i := 0; i < 120; i++ {
		if w := lookup(r, "192.0.2.9", `{"query":"user`+strconv.Itoa(i)+`"}`); w.Code != http.StatusOK {
			t.Fatalf("lookup %d: code=%d", i+1, w.Code)
		}
	}
	if w := lookup(r, "192.0.2.9", `{"query":"user-last"}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("121st lookup from one IP: code=%d", w.Code)
	}
	// the exhausted lookup bucket does not block the info endpoint.
	if w := isolirReq(r, http.MethodGet, "/public/isolir/info", "192.0.2.9", "", nil); w.Code != http.StatusOK {
		t.Fatalf("info after lookup limit: code=%d", w.Code)
	}
	if n := counter.sumPrefix("rl:isolir:ip:192.0.2.9:"); n != 121 {
		t.Fatalf("lookup IP bucket=%d", n)
	}
	if n := counter.sumPrefix("rl:isolir:info:ip:192.0.2.9:"); n != 1 {
		t.Fatalf("info IP bucket=%d", n)
	}
}

func TestInfo_OwnPerIPBucket300(t *testing.T) {
	counter := &isolirCounter{}
	r := isolirRouter(t, &fakePublicSvc{}, counter)
	for i := 0; i < 300; i++ {
		if w := isolirReq(r, http.MethodGet, "/public/isolir/info", "192.0.2.10", "", nil); w.Code != http.StatusOK {
			t.Fatalf("info %d: code=%d", i+1, w.Code)
		}
	}
	if w := isolirReq(r, http.MethodGet, "/public/isolir/info", "192.0.2.10", "", nil); w.Code != http.StatusTooManyRequests {
		t.Fatalf("301st info: code=%d", w.Code)
	}
	if n := counter.sumPrefix("rl:isolir:ip:"); n != 0 {
		t.Fatalf("info must not use the lookup IP bucket: %d", n)
	}
	if n := counter.sumPrefix("rl:isolir:info:ip:192.0.2.10:"); n != 301 {
		t.Fatalf("info IP bucket=%d", n)
	}
	if isolirInfoIPLimit != 300 || isolirIPLimit != 120 || isolirQueryLimit != 5 || isolirTenantLimit != 300 {
		t.Fatal("limit values changed")
	}
}

// fakeClock advances only when the service "works" or the handler sleeps.
type fakeClock struct {
	mu    sync.Mutex
	t     time.Time
	slept []time.Duration
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func (c *fakeClock) sleep(_ context.Context, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slept = append(c.slept, d)
	c.t = c.t.Add(d)
}

type slowPublicSvc struct {
	fakePublicSvc
	clock *fakeClock
	took  time.Duration
}

func (s *slowPublicSvc) Lookup(ctx context.Context, sc coretenant.Scope, q string, today time.Time) (service.PublicLookupResult, error) {
	s.clock.advance(s.took)
	return s.fakePublicSvc.Lookup(ctx, sc, q, today)
}

func floorRouter(t *testing.T, svc PublicService, clock *fakeClock) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mw, err := middleware.ResolvePublicOrganization(isolirHostResolver{t}, middleware.PublicHostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	h := NewIsolirPublicHandler(svc, nil)
	h.now, h.sleep = clock.now, clock.sleep
	h.RegisterRoutes(r.Group("", mw, middleware.RequireActiveTenant()))
	return r
}

func TestLookup_ResponseFloor(t *testing.T) {
	found := service.PublicLookupResult{Found: true, MaskedName: "B***"}
	cases := []struct {
		name   string
		took   time.Duration
		result service.PublicLookupResult
		want   []time.Duration
	}{
		{"fast uniform is delayed to the floor", 20 * time.Millisecond, service.PublicLookupResult{}, []time.Duration{280 * time.Millisecond}},
		{"fast found is delayed to the floor", 50 * time.Millisecond, found, []time.Duration{250 * time.Millisecond}},
		{"slow result is not delayed further", 450 * time.Millisecond, found, nil},
		{"exactly the floor is not delayed", lookupFloor, service.PublicLookupResult{}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clock := &fakeClock{t: time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC)}
			svc := &slowPublicSvc{fakePublicSvc: fakePublicSvc{result: c.result}, clock: clock, took: c.took}
			w := lookup(floorRouter(t, svc, clock), "203.0.113.5", `{"query":"budi"}`)
			if w.Code != http.StatusOK {
				t.Fatalf("code=%d", w.Code)
			}
			if !reflect.DeepEqual(clock.slept, c.want) {
				t.Fatalf("slept %v want %v", clock.slept, c.want)
			}
		})
	}
}

func TestLookup_ResponseFloorNotOnErrors(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC)}
	svc := &slowPublicSvc{fakePublicSvc: fakePublicSvc{err: errors.New("boom")}, clock: clock}
	if w := lookup(floorRouter(t, svc, clock), "203.0.113.5", `{"query":"budi"}`); w.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d", w.Code)
	}
	if len(clock.slept) != 0 {
		t.Fatalf("500 must not be delayed: %v", clock.slept)
	}
	// 429 comes from the limiter before the handler: never delayed.
	counter := &isolirCounter{}
	gin.SetMode(gin.TestMode)
	mw, _ := middleware.ResolvePublicOrganization(isolirHostResolver{t}, middleware.PublicHostOptions{})
	r := gin.New()
	h := NewIsolirPublicHandler(&fakePublicSvc{}, counter)
	h.now, h.sleep = clock.now, clock.sleep
	h.RegisterRoutes(r.Group("", mw, middleware.RequireActiveTenant()))
	var last *httptest.ResponseRecorder
	for i := 0; i < 6; i++ {
		last = lookup(r, "203.0.113."+strconv.Itoa(i+1), `{"query":"same"}`)
	}
	if last.Code != http.StatusTooManyRequests || len(clock.slept) != 5 {
		t.Fatalf("code=%d slept=%v (only the five 200s may sleep)", last.Code, clock.slept)
	}
}

func TestSleepCtx_ReturnsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	sleepCtx(ctx, time.Hour)
	if time.Since(start) > time.Second {
		t.Fatal("sleepCtx must return promptly when the context is cancelled")
	}
	start = time.Now()
	sleepCtx(context.Background(), 5*time.Millisecond)
	if time.Since(start) < 5*time.Millisecond {
		t.Fatal("sleepCtx returned before the duration")
	}
}

func TestInfo_DisabledReturnsNoTenant(t *testing.T) {
	r := isolirRouter(t, &fakePublicSvc{}, nil)
	w := isolirReq(r, http.MethodGet, "/public/isolir/info", "203.0.113.5", "", nil)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"success":true,"message":"ok","data":{"enabled":false}}` {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	svc := &fakePublicSvc{info: service.PublicInfo{Enabled: true, Tenant: &service.TenantContact{Name: "ISP", Phone: "0811", Email: "a@b.id"}}}
	w = isolirReq(isolirRouter(t, svc, nil), http.MethodGet, "/public/isolir/info", "203.0.113.5", "", nil)
	want := `{"success":true,"message":"ok","data":{"enabled":true,"tenant":{"name":"ISP","phone":"0811","email":"a@b.id"}}}`
	if strings.TrimSpace(w.Body.String()) != want {
		t.Fatalf("body=%s", w.Body.String())
	}
}

func TestLookup_ServiceErrorGeneric500(t *testing.T) {
	var bodies []string
	for _, e := range []error{errors.New("pq: connection refused for budi"), errors.New("router mk-1 timeout")} {
		w := lookup(isolirRouter(t, &fakePublicSvc{err: e}, nil), "203.0.113.5", `{"query":"budi"}`)
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "INTERNAL_ERROR") {
			t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
		}
		bodies = append(bodies, w.Body.String())
	}
	if bodies[0] != bodies[1] || strings.Contains(bodies[0], "budi") || strings.Contains(bodies[0], "mk-1") {
		t.Fatalf("500 bodies must be identical and generic: %v", bodies)
	}
}

func TestLookup_RateLimitKeySharedAcrossPhoneFormats(t *testing.T) {
	counter := &isolirCounter{}
	r := isolirRouter(t, &fakePublicSvc{}, counter)
	for i, q := range []string{"0812 3456 789", "0812-3456-789", "+62812 3456 789"} {
		lookup(r, "203.0.113."+string(rune('1'+i)), `{"query":"`+q+`"}`)
	}
	lookup(r, "203.0.113.9", `{"query":"budi"}`)
	var queryKeys []string
	for k, n := range counter.counts {
		if strings.HasPrefix(k, "rl:isolir:q:") {
			queryKeys = append(queryKeys, k)
			if strings.Contains(k, "0812") || strings.Contains(k, "62812") || strings.Contains(k, "budi") {
				t.Fatalf("raw value in key %q", k)
			}
			if n != 3 && n != 1 {
				t.Fatalf("key %q count=%d", k, n)
			}
		}
	}
	if len(queryKeys) != 2 {
		t.Fatalf("want 2 query buckets (phone, username), got %v", queryKeys)
	}
}
