package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/config"
	coretenant "zyad.cloud/internal/core/tenant"
	mikrotikhandler "zyad.cloud/internal/modules/mikrotik/handler"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
	receivablehandler "zyad.cloud/internal/modules/receivable/handler"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
)

type routesPublicSvc struct{ lookups, infos int }

func (s *routesPublicSvc) Info(context.Context, coretenant.Scope) (mikrotikservice.PublicInfo, error) {
	s.infos++
	return mikrotikservice.PublicInfo{}, nil
}

func (s *routesPublicSvc) Lookup(context.Context, coretenant.Scope, string, time.Time) (mikrotikservice.PublicLookupResult, error) {
	s.lookups++
	return mikrotikservice.PublicLookupResult{}, nil
}

type routesHostResolver struct{ t *testing.T }

func (h routesHostResolver) ResolvePublicHost(_ context.Context, host string) (coretenant.Context, bool, error) {
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

// The public isolir routes live only on the host-resolved public group: the
// host decides the tenant, an unknown host never reaches the service, and the
// authenticated /app/mikrotik group does not carry them.
func TestMikrotikPublicRoutesUnderHostResolver(t *testing.T) {
	svc := &routesPublicSvc{}
	deps := Dependencies{
		Logger:                slog.Default(),
		PublicHostResolver:    routesHostResolver{t},
		MikrotikPublicHandler: mikrotikhandler.NewIsolirPublicHandler(svc, nil),
	}
	engine, err := newRouter(deps)
	if err != nil {
		t.Fatalf("newRouter: %v", err)
	}

	seen := map[string]int{}
	for _, r := range engine.Routes() {
		seen[r.Method+" "+r.Path]++
		if strings.Contains(r.Path, "/public/isolir") && strings.Contains(r.Path, "/app/") {
			t.Fatalf("public isolir route under authenticated prefix: %s", r.Path)
		}
	}
	for _, want := range []string{"GET /api/v1/public/isolir/info", "POST /api/v1/public/isolir/lookup"} {
		if seen[want] != 1 {
			t.Fatalf("route %q registered %d times", want, seen[want])
		}
	}

	do := func(method, path, host string, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(`{"query":"x"}`))
		req.Host = host
		req.Header.Set("Content-Type", "application/json")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}

	// unauthenticated request on a registered tenant host works (no JWT needed).
	if w := do(http.MethodPost, "/api/v1/public/isolir/lookup", "isp.example.com", nil); w.Code != http.StatusOK || svc.lookups != 1 {
		t.Fatalf("lookup code=%d lookups=%d body=%s", w.Code, svc.lookups, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/v1/public/isolir/info", "isp.example.com", nil); w.Code != http.StatusOK || svc.infos != 1 {
		t.Fatalf("info code=%d infos=%d", w.Code, svc.infos)
	}
	// unknown host: rejected by the host resolver before the service.
	if w := do(http.MethodPost, "/api/v1/public/isolir/lookup", "unknown.example.com", nil); w.Code != http.StatusNotFound || svc.lookups != 1 {
		t.Fatalf("unknown host code=%d lookups=%d", w.Code, svc.lookups)
	}
	// selector header is refused on the public group.
	if w := do(http.MethodPost, "/api/v1/public/isolir/lookup", "isp.example.com",
		map[string]string{"X-Organization-ID": "22222222-2222-4222-8222-222222222222"}); w.Code != http.StatusBadRequest || svc.lookups != 1 {
		t.Fatalf("selector code=%d lookups=%d", w.Code, svc.lookups)
	}
}

type routesInvoiceSvc struct{}

func (routesInvoiceSvc) View(context.Context, string) (receivableservice.PublicInvoiceView, error) {
	return receivableservice.PublicInvoiceView{}, receivableservice.ErrLinkInvalid
}

func (routesInvoiceSvc) PDF(context.Context, string) (receivableservice.InvoicePDF, error) {
	return receivableservice.InvoicePDF{}, receivableservice.ErrLinkInvalid
}

func (routesInvoiceSvc) Resolve(context.Context, string) (receivableservice.ResolvedInvoice, error) {
	return receivableservice.ResolvedInvoice{}, receivableservice.ErrLinkInvalid
}

// The provisioning endpoint exists on the fully built router only when the
// mikrotik adapter is wired into the receivable handler (as app.go does).
func TestPublicInvoiceProvisioningRouteWiring(t *testing.T) {
	m := buildMikrotik(config.Config{}, nil, mikrotikBuild{
		Accounts: &fakeAccounts{}, Contacts: &fakeContacts{}, Products: fakeProducts{},
	})
	const route = "GET /api/v1/public/invoices/:token/provisioning"
	for _, wired := range []bool{true, false} {
		h := receivablehandler.NewPublicInvoiceHandler(routesInvoiceSvc{}, nil)
		if wired {
			h.WithProvisioning(m.Provisioning)
		}
		engine, err := newRouter(Dependencies{Logger: slog.Default(), ReceivablePublicInvoiceHandler: h})
		if err != nil {
			t.Fatalf("newRouter: %v", err)
		}
		found := false
		for _, r := range engine.Routes() {
			if r.Method+" "+r.Path == route {
				found = true
			}
		}
		if found != wired {
			t.Fatalf("wired=%v: route registered=%v", wired, found)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/public/invoices/tok/provisioning", nil))
		// Wired: the handler runs and the unknown token is the generic 404 LINK_INVALID.
		// Not wired: gin's own 404 (no route).
		if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "LINK_INVALID") != wired {
			t.Fatalf("wired=%v: code=%d body=%s", wired, w.Code, w.Body.String())
		}
	}
}
