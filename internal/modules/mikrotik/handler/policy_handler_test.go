package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/service"
)

const testProductID = "66666666-6666-6666-6666-666666666666"

type fakePolicyService struct {
	err error

	exempt  bool
	reason  string
	enabled bool
	calls   int
}

func (f *fakePolicyService) SetExempt(_ context.Context, _ coretenant.Scope, accountID string, exempt bool, reason, _ string) (domain.CustomerPolicy, error) {
	f.calls++
	f.exempt, f.reason = exempt, reason
	return domain.CustomerPolicy{AccountID: accountID, Exempt: exempt, Reason: reason, SetAt: time.Now()}, f.err
}
func (f *fakePolicyService) GetPolicy(_ context.Context, _ coretenant.Scope, accountID string) (domain.CustomerPolicy, error) {
	f.calls++
	return domain.CustomerPolicy{AccountID: accountID}, f.err
}
func (f *fakePolicyService) PreviewTrigger(_ context.Context, _ coretenant.Scope, productID string) (service.TriggerPreview, error) {
	f.calls++
	return service.TriggerPreview{
		Product: service.ProductInfo{ID: productID, Name: "Paket 10M", Recurring: true, Active: true},
		Usage:   service.ProductUsage{ActiveContracts: 4, Accounts: 3, OverdueInvoices: 1}, EffectiveFrom: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
	}, f.err
}
func (f *fakePolicyService) SetTrigger(_ context.Context, _ coretenant.Scope, productID string, enabled bool, _ string) (domain.ProductTrigger, error) {
	f.calls++
	f.enabled = enabled
	return domain.ProductTrigger{ProductID: productID, Enabled: enabled, EffectiveFrom: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)}, f.err
}
func (f *fakePolicyService) UnmarkedProducts(context.Context, coretenant.Scope) ([]service.ProductInfo, error) {
	f.calls++
	return []service.ProductInfo{{ID: testProductID, Name: "Paket 10M", Recurring: true, Active: true}}, f.err
}

func policyRouter(t *testing.T, svc PolicyService) *gin.Engine {
	r := newTestRouter(t)
	NewPolicyHandler(svc).RegisterRoutes(r.Group("/app/mikrotik"), nil, nil)
	return r
}

func TestPolicyRoutes_Success(t *testing.T) {
	const base = "/app/mikrotik"
	for _, tc := range []struct{ name, method, path, body string }{
		{"get policy", http.MethodGet, base + "/customers/" + testAccountID + "/policy", ``},
		{"set policy", http.MethodPut, base + "/customers/" + testAccountID + "/policy", `{"exempt":true,"reason":"pelanggan VIP"}`},
		{"unmarked", http.MethodGet, base + "/products/unmarked", ``},
		{"preview", http.MethodGet, base + "/products/" + testProductID + "/trigger/preview", ``},
		{"set trigger", http.MethodPut, base + "/products/" + testProductID + "/trigger", `{"enabled":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := performJSON(policyRouter(t, &fakePolicyService{}), tc.method, tc.path, tc.body)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", w.Code, w.Body)
			}
			assertNoCredentialKeys(t, w.Body.Bytes())
		})
	}
}

func TestPolicyRoutes_Bodies(t *testing.T) {
	svc := &fakePolicyService{}
	r := policyRouter(t, svc)
	w := performJSON(r, http.MethodPut, "/app/mikrotik/customers/"+testAccountID+"/policy", `{"exempt":true,"reason":"VIP"}`)
	if w.Code != http.StatusOK || !svc.exempt || svc.reason != "VIP" {
		t.Fatalf("status = %d svc = %+v", w.Code, svc)
	}
	w = performJSON(r, http.MethodPut, "/app/mikrotik/products/"+testProductID+"/trigger", `{"enabled":true}`)
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !svc.enabled || body.Data["effective_from"] != "2026-10-31" || body.Data["enabled"] != true {
		t.Fatalf("data = %v svc = %+v", body.Data, svc)
	}
	w = performJSON(r, http.MethodGet, "/app/mikrotik/products/"+testProductID+"/trigger/preview", ``)
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data["overdue_invoices"] != float64(1) || body.Data["active_contracts"] != float64(4) || body.Data["already_enabled"] != false {
		t.Fatalf("preview = %v", body.Data)
	}
}

func TestPolicyRoutes_ErrorMapping(t *testing.T) {
	const base = "/app/mikrotik"
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"invalid input", &mikrotik.InvalidInputError{Message: "reason is required"}, http.StatusUnprocessableEntity},
		{"account not found", mikrotik.ErrAccountNotFound, http.StatusNotFound},
		{"product not found", mikrotik.ErrProductNotFound, http.StatusNotFound},
		{"unknown", errors.New("pq: boom"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := policyRouter(t, &fakePolicyService{err: tc.err})
			for _, rt := range []struct{ method, path, body string }{
				{http.MethodGet, base + "/customers/" + testAccountID + "/policy", ``},
				{http.MethodPut, base + "/customers/" + testAccountID + "/policy", `{"exempt":false}`},
				{http.MethodGet, base + "/products/unmarked", ``},
				{http.MethodGet, base + "/products/" + testProductID + "/trigger/preview", ``},
				{http.MethodPut, base + "/products/" + testProductID + "/trigger", `{"enabled":false}`},
			} {
				if w := performJSON(r, rt.method, rt.path, rt.body); w.Code != tc.want {
					t.Errorf("%s %s: status = %d, want %d, body = %s", rt.method, rt.path, w.Code, tc.want, w.Body)
				}
			}
		})
	}
}

func TestPolicyBodies_RejectUnknownAndInvalid(t *testing.T) {
	const acc = "/app/mikrotik/customers/" + testAccountID + "/policy"
	const trig = "/app/mikrotik/products/" + testProductID + "/trigger"
	for _, tc := range []struct{ name, path, body string }{
		{"policy unknown field", acc, `{"exempt":true,"reason":"x","extra":1}`},
		{"policy missing exempt", acc, `{"reason":"x"}`},
		{"policy reason too long", acc, `{"exempt":true,"reason":"` + repeat("a", 501) + `"}`},
		{"policy empty body", acc, ``},
		{"trigger unknown field", trig, `{"enabled":true,"force":true}`},
		{"trigger missing enabled", trig, `{}`},
		{"trigger wrong type", trig, `{"enabled":"yes"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakePolicyService{}
			w := performJSON(policyRouter(t, svc), http.MethodPut, tc.path, tc.body)
			if w.Code != http.StatusUnprocessableEntity || svc.calls != 0 {
				t.Fatalf("status = %d calls = %d body = %s", w.Code, svc.calls, w.Body)
			}
		})
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

func TestPolicyRoutes_MalformedIDIs404(t *testing.T) {
	svc := &fakePolicyService{}
	r := policyRouter(t, svc)
	for _, rt := range []struct{ method, path, body string }{
		{http.MethodGet, "/app/mikrotik/customers/x/policy", ``},
		{http.MethodPut, "/app/mikrotik/customers/x/policy", `{"exempt":false}`},
		{http.MethodGet, "/app/mikrotik/products/x/trigger/preview", ``},
		{http.MethodPut, "/app/mikrotik/products/x/trigger", `{"enabled":true}`},
	} {
		if w := performJSON(r, rt.method, rt.path, rt.body); w.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d", rt.method, rt.path, w.Code)
		}
	}
	if svc.calls != 0 {
		t.Fatal("service must not be called")
	}
}

func TestPolicyRoutesRequirePermissions(t *testing.T) {
	routes := []struct{ method, path, perm string }{
		{http.MethodGet, "/app/mikrotik/customers/:account_id/policy", "mikrotik_policy.read"},
		{http.MethodPut, "/app/mikrotik/customers/:account_id/policy", "mikrotik_policy.update"},
		{http.MethodGet, "/app/mikrotik/products/unmarked", "mikrotik_product_trigger.read"},
		{http.MethodGet, "/app/mikrotik/products/:id/trigger/preview", "mikrotik_product_trigger.read"},
		{http.MethodPut, "/app/mikrotik/products/:id/trigger", "mikrotik_product_trigger.update"},
	}
	assertRoutesRequirePermissions(t, routes, func(g *gin.RouterGroup, p *recordingChecker) {
		NewPolicyHandler(&fakePolicyService{}).RegisterRoutes(g, p, nil)
	})
}
