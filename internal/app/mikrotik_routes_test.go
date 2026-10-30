package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	mikrotikdomain "zyad.cloud/internal/modules/mikrotik/domain"
	mikrotikhandler "zyad.cloud/internal/modules/mikrotik/handler"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
)

type recordingEntitlements struct {
	allow bool
	keys  []string
}

func (r *recordingEntitlements) RequireFeature(_ context.Context, _ string, key string) error {
	r.keys = append(r.keys, key)
	if r.allow {
		return nil
	}
	return coreerrors.New("ENTITLEMENT_REQUIRED", "feature entitlement is required", http.StatusForbidden)
}

type countingRouterService struct{ calls int }

func (s *countingRouterService) Register(context.Context, coretenant.Scope, mikrotikservice.RegisterInput) (mikrotikdomain.Router, error) {
	s.calls++
	return mikrotikdomain.Router{}, nil
}
func (s *countingRouterService) Get(context.Context, coretenant.Scope, string) (mikrotikdomain.Router, error) {
	s.calls++
	return mikrotikdomain.Router{}, nil
}
func (s *countingRouterService) List(context.Context, coretenant.Scope, int, int) ([]mikrotikdomain.Router, int64, error) {
	s.calls++
	return nil, 0, nil
}
func (s *countingRouterService) Check(context.Context, coretenant.Scope, string) (mikrotikdomain.Router, error) {
	s.calls++
	return mikrotikdomain.Router{}, nil
}
func (s *countingRouterService) Update(context.Context, coretenant.Scope, string, mikrotikservice.UpdateInput) (mikrotikdomain.Router, error) {
	s.calls++
	return mikrotikdomain.Router{}, nil
}
func (s *countingRouterService) Delete(context.Context, coretenant.Scope, string, string) error {
	s.calls++
	return nil
}

func TestMikrotikRoutesAreGatedByEntitlement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     "11111111-1111-1111-1111-111111111111",
		OrganizationSlug:   "organization-a",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID:       "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("tenant context: %v", err)
	}

	for _, tc := range []struct {
		name       string
		allow      bool
		wantStatus int
		wantCalls  int
	}{
		{"denied entitlement blocks the group", false, http.StatusForbidden, 0},
		{"granted entitlement reaches the handler", true, http.StatusOK, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checker := &recordingEntitlements{allow: tc.allow}
			svc := &countingRouterService{}
			engine := gin.New()
			protected := engine.Group("/api/v1")
			protected.Use(func(c *gin.Context) {
				c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tenantContext))
				c.Next()
			})
			// PermissionChecker nil: RBAC is covered in the handler package.
			registerMikrotikRoutes(protected, Dependencies{
				CRMEntitlementChecker: checker,
				MikrotikRouterHandler: mikrotikhandler.NewRouterHandler(svc),
			})

			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/app/mikrotik/routers", nil))

			if w.Code != tc.wantStatus || svc.calls != tc.wantCalls {
				t.Fatalf("status = %d (want %d), service calls = %d (want %d), body = %s", w.Code, tc.wantStatus, svc.calls, tc.wantCalls, w.Body)
			}
			if len(checker.keys) != 1 || checker.keys[0] != "mikrotik.enabled" {
				t.Fatalf("entitlement keys asked = %v, want [mikrotik.enabled]", checker.keys)
			}
		})
	}
}

type countingSetupService struct{ calls int }

func (s *countingSetupService) Preview(context.Context, coretenant.Scope, string, mikrotikservice.SetupInput) (mikrotikdomain.SetupPlan, error) {
	s.calls++
	return mikrotikdomain.SetupPlan{}, nil
}
func (s *countingSetupService) Apply(context.Context, coretenant.Scope, string, mikrotikservice.SetupInput, string, string) (mikrotikdomain.Router, error) {
	s.calls++
	return mikrotikdomain.Router{}, nil
}
func (s *countingSetupService) Cleanup(context.Context, coretenant.Scope, string, string) (mikrotikdomain.Router, error) {
	s.calls++
	return mikrotikdomain.Router{}, nil
}

func TestMikrotikSetupRoutesAreGatedByEntitlement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "11111111-1111-1111-1111-111111111111", OrganizationSlug: "organization-a",
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", MembershipStatus: "active", MembershipVersion: 1,
		ResolutionSource: coretenant.ResolutionSourceSession, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, allow := range []bool{false, true} {
		checker := &recordingEntitlements{allow: allow}
		setup := &countingSetupService{}
		engine := gin.New()
		protected := engine.Group("/api/v1")
		protected.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tenantContext))
			c.Next()
		})
		registerMikrotikRoutes(protected, Dependencies{
			CRMEntitlementChecker: checker,
			MikrotikRouterHandler: mikrotikhandler.NewRouterHandler(&countingRouterService{}),
			MikrotikSetupHandler:  mikrotikhandler.NewSetupHandler(setup),
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/app/mikrotik/routers/33333333-3333-3333-3333-333333333333/setup/cleanup", nil)
		engine.ServeHTTP(w, req)
		wantCalls, wantStatus := 0, http.StatusForbidden
		if allow {
			wantCalls, wantStatus = 1, http.StatusOK
		}
		if setup.calls != wantCalls || w.Code != wantStatus {
			t.Fatalf("allow=%v: status = %d (want %d), calls = %d (want %d)", allow, w.Code, wantStatus, setup.calls, wantCalls)
		}
	}
}
