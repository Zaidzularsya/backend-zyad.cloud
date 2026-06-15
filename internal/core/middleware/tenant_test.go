package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"

	"github.com/gin-gonic/gin"
)

func TestSetTenantContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     "organization-1",
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID:       "membership-1",
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}

	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		if !SetTenantContext(c, tenantContext) {
			t.Fatal("SetTenantContext() returned false")
		}
		if TenantID(c) != "organization-1" {
			t.Fatalf("TenantID() = %q", TenantID(c))
		}
		if _, ok := coretenant.FromContext(c.Request.Context()); !ok {
			t.Fatal("standard request context does not contain tenant context")
		}
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestResolveAuthenticatedOrganizationSetsVerifiedContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := fakeOrganizationResolver{tenantContext: mustTenantContext(t)}

	router := gin.New()
	router.GET(
		"/",
		seedAuthenticatedUser(AuthenticatedUser{
			ID:        "user-1",
			SessionID: "session-1",
		}),
		ResolveAuthenticatedOrganization(resolver),
		func(c *gin.Context) {
			tenantContext, err := RequireTenantContext(c)
			if err != nil || tenantContext.OrganizationID() != "organization-1" {
				t.Fatalf("RequireTenantContext() = %#v, %v", tenantContext, err)
			}
			c.Status(http.StatusNoContent)
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(OrganizationIDHeader, "organization-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestResolveAuthenticatedOrganizationReturnsResolverError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := fakeOrganizationResolver{
		err: coreerrors.New("ORGANIZATION_ACCESS_DENIED", "denied", http.StatusForbidden),
	}

	router := gin.New()
	router.GET(
		"/",
		seedAuthenticatedUser(AuthenticatedUser{
			ID:        "user-1",
			SessionID: "session-1",
		}),
		ResolveAuthenticatedOrganization(resolver),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequireActiveTenantFailsClosedWithoutContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(
		"/",
		RequireActiveTenant(),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequireActiveTenantRejectsSuspendedOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(
		"/",
		setTenantContextMiddleware(mustTenantContextWith(
			t,
			coretenant.OrganizationTypeCustomer,
			coretenant.OrganizationStatusSuspended,
			coretenant.ResolutionSourceCustomDomain,
		)),
		RequireActiveTenant(),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequireSetupTenantAllowsProvisioningOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []coretenant.OrganizationStatus{
		coretenant.OrganizationStatusPending,
		coretenant.OrganizationStatusProvisioning,
		coretenant.OrganizationStatusProvisioningFailed,
	} {
		t.Run(string(status), func(t *testing.T) {
			router := gin.New()
			router.GET(
				"/",
				setTenantContextMiddleware(mustTenantContextWith(
					t,
					coretenant.OrganizationTypeCustomer,
					status,
					coretenant.ResolutionSourceInternal,
				)),
				RequireSetupTenant(),
				func(c *gin.Context) { c.Status(http.StatusNoContent) },
			)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d", rec.Code)
			}
		})
	}

	router := gin.New()
	router.GET(
		"/",
		setTenantContextMiddleware(mustTenantContextWith(
			t,
			coretenant.OrganizationTypeCustomer,
			coretenant.OrganizationStatusActive,
			coretenant.ResolutionSourceInternal,
		)),
		RequireSetupTenant(),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("active organization setup status = %d", rec.Code)
	}
}

func TestTenantTypeGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name             string
		organizationType coretenant.OrganizationType
		guard            gin.HandlerFunc
		wantStatus       int
	}{
		{
			name:             "platform allowed",
			organizationType: coretenant.OrganizationTypePlatform,
			guard:            RequirePlatformTenant(),
			wantStatus:       http.StatusNoContent,
		},
		{
			name:             "customer rejected by platform guard",
			organizationType: coretenant.OrganizationTypeCustomer,
			guard:            RequirePlatformTenant(),
			wantStatus:       http.StatusForbidden,
		},
		{
			name:             "customer allowed",
			organizationType: coretenant.OrganizationTypeCustomer,
			guard:            RequireCustomerTenant(),
			wantStatus:       http.StatusNoContent,
		},
		{
			name:             "platform rejected by customer guard",
			organizationType: coretenant.OrganizationTypePlatform,
			guard:            RequireCustomerTenant(),
			wantStatus:       http.StatusForbidden,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET(
				"/",
				setTenantContextMiddleware(mustTenantContextWith(
					t,
					test.organizationType,
					coretenant.OrganizationStatusActive,
					coretenant.ResolutionSourceInternal,
				)),
				test.guard,
				func(c *gin.Context) { c.Status(http.StatusNoContent) },
			)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if rec.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, test.wantStatus)
			}
		})
	}
}

func TestSetTenantContextAttachesStructuredFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET(
		"/",
		seedAuthenticatedUser(AuthenticatedUser{
			ID:        "user-1",
			SessionID: "session-1",
		}),
		setTenantContextMiddleware(mustTenantContext(t)),
		func(c *gin.Context) {
			fields := TenantLogFields(c)
			if fields["organization_id"] != "organization-1" ||
				fields["membership_id"] != "membership-1" ||
				fields["resolution_source"] != coretenant.ResolutionSourceHeader ||
				fields["user_id"] != "user-1" ||
				fields["session_id"] != "session-1" ||
				fields["request_id"] == "" {
				t.Fatalf("TenantLogFields() = %#v", fields)
			}
			for _, key := range []string{
				OrganizationIDContextKey,
				OrganizationSlugContextKey,
				OrganizationTypeContextKey,
				OrganizationStatusContextKey,
				MembershipIDContextKey,
				ResolutionSourceContextKey,
				DataPlacementContextKey,
				ImpersonationSessionIDContextKey,
			} {
				if _, exists := c.Get(key); !exists {
					t.Fatalf("missing context field %q", key)
				}
			}
			c.Status(http.StatusNoContent)
		},
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestOrganizationSelectorPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		organizationID, header := OrganizationSelector(c)
		if organizationID != "canonical" {
			t.Fatalf("organizationID = %q", organizationID)
		}
		if header != OrganizationIDHeader {
			t.Fatalf("header = %q", header)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(LegacyOrganizationIDHeader, "legacy")
	req.Header.Set(OrganizationIDHeader, "canonical")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestOrganizationSelectorSupportsLegacyHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, header := range []string{LegacyOrganizationIDHeader, LegacyTenantIDHeader} {
		t.Run(header, func(t *testing.T) {
			router := gin.New()
			router.GET("/", func(c *gin.Context) {
				organizationID, gotHeader := OrganizationSelector(c)
				if organizationID != "legacy-organization" || gotHeader != header {
					t.Fatalf("selector = %q, %q", organizationID, gotHeader)
				}
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(header, "legacy-organization")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d", rec.Code)
			}
		})
	}
}

type fakeOrganizationResolver struct {
	tenantContext coretenant.Context
	resolved      bool
	err           error
}

func (f fakeOrganizationResolver) ResolveAuthenticatedOrganization(
	context.Context,
	string,
	string,
	string,
	string,
) (coretenant.Context, bool, error) {
	if f.err != nil {
		return coretenant.Context{}, false, f.err
	}
	resolved := f.resolved
	if f.tenantContext.IsValid() {
		resolved = true
	}
	return f.tenantContext, resolved, nil
}

func mustTenantContext(t *testing.T) coretenant.Context {
	t.Helper()
	return mustTenantContextWith(
		t,
		coretenant.OrganizationTypeCustomer,
		coretenant.OrganizationStatusActive,
		coretenant.ResolutionSourceHeader,
	)
}

func mustTenantContextWith(
	t *testing.T,
	organizationType coretenant.OrganizationType,
	organizationStatus coretenant.OrganizationStatus,
	resolutionSource coretenant.ResolutionSource,
) coretenant.Context {
	t.Helper()
	input := coretenant.VerifiedContextInput{
		OrganizationID:     "organization-1",
		OrganizationSlug:   "acme",
		OrganizationType:   organizationType,
		OrganizationStatus: organizationStatus,
		ResolutionSource:   resolutionSource,
		DataPlacement:      coretenant.DataPlacementShared,
	}
	if resolutionSource == coretenant.ResolutionSourceSession ||
		resolutionSource == coretenant.ResolutionSourceHeader ||
		resolutionSource == coretenant.ResolutionSourceMembership {
		input.MembershipID = "membership-1"
		input.MembershipStatus = "active"
		input.MembershipVersion = 1
	}
	tenantContext, err := coretenant.NewVerifiedContext(input)
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	return tenantContext
}

func setTenantContextMiddleware(tenantContext coretenant.Context) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !SetTenantContext(c, tenantContext) {
			panic("invalid tenant context in test")
		}
		c.Next()
	}
}
