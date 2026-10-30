package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/service"
)

// deadlineRecorder is a ResponseWriter that supports SetWriteDeadline, like
// the server's real connection writer.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	d.deadlines = append(d.deadlines, t)
	return nil
}

func serveWithDeadline(h http.Handler, method, path, body string) *deadlineRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(w, req)
	return w
}

func TestLongRoutesExtendWriteDeadline(t *testing.T) {
	const id = "22222222-2222-2222-2222-222222222222"
	routerSvc := &fakeService{
		registerFn: func(service.RegisterInput) (domain.Router, error) { return sampleRouter(), nil },
		checkFn:    func(string) (domain.Router, error) { return sampleRouter(), nil },
		getFn:      func(string) (domain.Router, error) { return sampleRouter(), nil },
	}
	engine := newTestRouter(t)
	NewRouterHandler(routerSvc).RegisterRoutes(engine.Group("/app/mikrotik"), nil)
	NewSetupHandler(&fakeSetupService{
		previewFn: func(service.SetupInput) (domain.SetupPlan, error) { return domain.SetupPlan{}, nil },
		applyFn:   func(service.SetupInput, string) (domain.Router, error) { return sampleRouter(), nil },
		cleanupFn: func() (domain.Router, error) { return sampleRouter(), nil },
	}).RegisterRoutes(engine.Group("/app/mikrotik"), nil)

	setupBody := `{"pool_cidr":"10.255.254.0/24","platform_host":"zyad.online"}`
	cases := []struct {
		method, path, body string
		want               time.Duration
	}{
		{http.MethodPost, "/app/mikrotik/routers", `{"name":"a","host":"10.0.0.1","port":8728,"username":"u","password":"p"}`, probeWriteDeadline},
		{http.MethodPatch, "/app/mikrotik/routers/" + id, `{"name":"b"}`, probeWriteDeadline},
		{http.MethodPost, "/app/mikrotik/routers/" + id + "/check", ``, probeWriteDeadline},
		{http.MethodPost, "/app/mikrotik/routers/" + id + "/setup/preview", setupBody, previewWriteDeadline},
		{http.MethodPost, "/app/mikrotik/routers/" + id + "/setup/apply", `{"pool_cidr":"10.255.254.0/24","platform_host":"zyad.online","plan_hash":"h"}`, setupWriteDeadline},
		{http.MethodPost, "/app/mikrotik/routers/" + id + "/setup/cleanup", ``, setupWriteDeadline},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			start := time.Now()
			w := serveWithDeadline(engine, tc.method, tc.path, tc.body)
			if w.Code >= 500 {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if len(w.deadlines) != 1 {
				t.Fatalf("SetWriteDeadline calls = %d, want 1", len(w.deadlines))
			}
			got := w.deadlines[0].Sub(start)
			if got < tc.want-time.Second || got > tc.want+time.Second {
				t.Fatalf("deadline = now+%v, want about now+%v", got, tc.want)
			}
		})
	}

	// Short routes keep the server default.
	w := serveWithDeadline(engine, http.MethodGet, "/app/mikrotik/routers/"+id, "")
	if len(w.deadlines) != 0 {
		t.Fatalf("GET one must not extend the deadline, got %d calls", len(w.deadlines))
	}
}

func TestUnsupportedWriteDeadlineDoesNotFailRequest(t *testing.T) {
	r := routerFor(t, &fakeService{checkFn: func(string) (domain.Router, error) { return sampleRouter(), nil }})
	// httptest.ResponseRecorder does not support SetWriteDeadline.
	w := performJSON(r, http.MethodPost, "/app/mikrotik/routers/22222222-2222-2222-2222-222222222222/check", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
}
