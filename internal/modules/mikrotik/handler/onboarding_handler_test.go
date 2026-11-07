package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/service"
)

type fakeOnboardingService struct {
	summary service.OnboardingSummary
	gotIDs  []string
	calls   int
}

func (f *fakeOnboardingService) Summary(context.Context, coretenant.Scope) (service.OnboardingSummary, error) {
	return f.summary, nil
}

func (f *fakeOnboardingService) CustomerStatuses(_ context.Context, _ coretenant.Scope, ids []string) ([]service.CustomerStatus, error) {
	f.calls++
	f.gotIDs = ids
	out := make([]service.CustomerStatus, 0, len(ids))
	for i, id := range ids {
		state := []string{"normal", "isolated", "none"}[i%3]
		st := service.CustomerStatus{AccountID: id, IsolationState: state, Exempt: i == 0, Secrets: i}
		if i == 0 {
			st.ExemptedUntil = "2026-11-30"
		}
		out = append(out, st)
	}
	return out, nil
}

const (
	uuidA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa1"
	uuidB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbb2"
	uuidC = "cccccccc-cccc-cccc-cccc-ccccccccccc3"
)

func onboardingEngine(t *testing.T, svc *fakeOnboardingService) http.Handler {
	r := newTestRouter(t)
	NewOnboardingHandler(svc).RegisterRoutes(r.Group("/app/mikrotik"), nil, nil)
	return r
}

func TestOnboarding_HandlerReturnsSummary(t *testing.T) {
	svc := &fakeOnboardingService{summary: service.OnboardingSummary{RouterRegistered: true, SecretsPaired: 2, Complete: true}}
	w := performJSON(onboardingEngine(t, svc), http.MethodGet, "/app/mikrotik/onboarding", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"router_registered", "router_ready", "secrets_total", "secrets_paired", "secrets_needs_profile", "products_marked", "sender_configured", "complete"} {
		if _, ok := body.Data[k]; !ok {
			t.Errorf("missing key %s in %v", k, body.Data)
		}
	}
	if body.Data["complete"] != true || body.Data["secrets_paired"] != float64(2) {
		t.Fatalf("data = %v", body.Data)
	}
}

func TestCustomersStatus_NoneIsolatedNormal(t *testing.T) {
	svc := &fakeOnboardingService{}
	// uuidA repeated: deduplicated, order of first appearance kept; mixed case accepted.
	url := fmt.Sprintf("/app/mikrotik/customers/status?account_ids=%s,%s,%s,%s", uuidA, uuidB, strings.ToUpper(uuidC), uuidA)
	w := performJSON(onboardingEngine(t, svc), http.MethodGet, url, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if len(svc.gotIDs) != 3 || svc.gotIDs[0] != uuidA || svc.gotIDs[1] != uuidB || svc.gotIDs[2] != uuidC {
		t.Fatalf("ids passed to service = %v", svc.gotIDs)
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 3 {
		t.Fatalf("data = %v", body.Data)
	}
	wantStates := []string{"normal", "isolated", "none"}
	if body.Data[0]["exempted_until"] != "2026-11-30" || body.Data[1]["exempted_until"] != nil {
		t.Errorf("exempted_until = %v / %v", body.Data[0]["exempted_until"], body.Data[1]["exempted_until"])
	}
	for i, row := range body.Data {
		if row["pending_isolation"] != false {
			t.Errorf("row %d pending_isolation = %v", i, row["pending_isolation"])
		}
		if row["isolation_state"] != wantStates[i] {
			t.Errorf("row %d state = %v", i, row["isolation_state"])
		}
		for _, k := range []string{"account_id", "exempt", "secrets", "exempted_until", "pending_isolation"} {
			if _, ok := row[k]; !ok {
				t.Errorf("row %d missing %s", i, k)
			}
		}
	}
}

func TestCustomersStatus_RejectsOver100AndBadUUID(t *testing.T) {
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = fmt.Sprintf("00000000-0000-0000-0000-%012d", i)
	}
	exactly100 := strings.Join(ids[:100], ",")
	cases := []struct {
		name, query string
		want        int
	}{
		{"101 ids", strings.Join(ids, ","), http.StatusUnprocessableEntity},
		{"bad uuid", uuidA + ",not-a-uuid", http.StatusUnprocessableEntity},
		{"empty", "", http.StatusUnprocessableEntity},
		{"blank element", uuidA + ",,", http.StatusUnprocessableEntity},
		{"exactly 100", exactly100, http.StatusOK},
		{"101 with duplicates dedupes to 100", exactly100 + "," + ids[0], http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeOnboardingService{}
			w := performJSON(onboardingEngine(t, svc), http.MethodGet, "/app/mikrotik/customers/status?account_ids="+tc.query, "")
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.want, w.Body)
			}
			if tc.want != http.StatusOK && svc.calls != 0 {
				t.Fatal("service must not be called on invalid input")
			}
		})
	}
}

func TestOnboardingRoutesRequirePermissions(t *testing.T) {
	routes := []struct{ method, path, perm string }{
		{http.MethodGet, "/app/mikrotik/onboarding", "mikrotik_router.read"},
		{http.MethodGet, "/app/mikrotik/customers/status", "mikrotik_policy.read"},
	}
	for _, rt := range routes {
		checker := &recordingChecker{}
		r := newTestRouter(t)
		r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "user-1") })
		NewOnboardingHandler(&fakeOnboardingService{}).RegisterRoutes(r.Group("/app/mikrotik"), checker, nil)
		w := performJSON(r, rt.method, rt.path, "")
		if w.Code == http.StatusOK || len(checker.asked) == 0 || checker.asked[0] != rt.perm {
			t.Errorf("%s: status %d asked %v, want denial asking %s", rt.path, w.Code, checker.asked, rt.perm)
		}
	}
}
