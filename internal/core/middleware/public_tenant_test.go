package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"

	"github.com/gin-gonic/gin"
)

type fakePublicHostResolver struct {
	tenantContext coretenant.Context
	resolvedHost  string
	resolved      bool
	err           error
}

func (f *fakePublicHostResolver) ResolvePublicHost(
	_ context.Context,
	host string,
) (coretenant.Context, bool, error) {
	f.resolvedHost = host
	return f.tenantContext, f.resolved, f.err
}

func TestResolvePublicOrganizationUsesForwardedHostFromTrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := &fakePublicHostResolver{
		tenantContext: mustPublicTenantContext(t),
		resolved:      true,
	}
	middleware, err := ResolvePublicOrganization(resolver, PublicHostOptions{
		TrustForwardedHost: true,
		TrustedProxyCIDRs:  []string{"10.0.0.0/8"},
	})
	if err != nil {
		t.Fatalf("ResolvePublicOrganization() error = %v", err)
	}

	router := gin.New()
	router.GET("/", middleware, func(c *gin.Context) {
		if _, err := RequireTenantContext(c); err != nil {
			t.Fatalf("RequireTenantContext() error = %v", err)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "http://internal:8080/", nil)
	req.RemoteAddr = "10.1.2.3:4567"
	req.Header.Set("X-Forwarded-Host", "tenant.example.test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent || resolver.resolvedHost != "tenant.example.test" {
		t.Fatalf("status/host = %d, %q", rec.Code, resolver.resolvedHost)
	}
}

func TestResolvePublicOrganizationIgnoresForwardedHostFromUntrustedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := &fakePublicHostResolver{}
	middleware, err := ResolvePublicOrganization(resolver, PublicHostOptions{
		TrustForwardedHost: true,
		TrustedProxyCIDRs:  []string{"10.0.0.0/8"},
	})
	if err != nil {
		t.Fatalf("ResolvePublicOrganization() error = %v", err)
	}

	router := gin.New()
	router.GET("/", middleware, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "http://origin.example.test/", nil)
	req.RemoteAddr = "192.0.2.10:4567"
	req.Header.Set("X-Forwarded-Host", "evil.example.test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent || resolver.resolvedHost != "origin.example.test" {
		t.Fatalf("status/host = %d, %q", rec.Code, resolver.resolvedHost)
	}
}

func TestResolvePublicOrganizationRejectsOrganizationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	middleware, err := ResolvePublicOrganization(
		&fakePublicHostResolver{},
		PublicHostOptions{},
	)
	if err != nil {
		t.Fatalf("ResolvePublicOrganization() error = %v", err)
	}

	router := gin.New()
	router.GET("/", middleware, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "http://tenant.example.test/", nil)
	req.Header.Set(OrganizationIDHeader, "organization-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func mustPublicTenantContext(t *testing.T) coretenant.Context {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     "organization-1",
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceCustomDomain,
		DataPlacement:      coretenant.DataPlacementShared,
		RequestHost:        "tenant.example.test",
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	return tenantContext
}
