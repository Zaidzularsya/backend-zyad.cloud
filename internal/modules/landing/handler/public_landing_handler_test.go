package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	coremiddleware "zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/service"
)

type publicResolverServiceStub struct {
	scope        coretenant.Scope
	slug         string
	preview      string
	customHost   string
	usedDomain   bool
	resolvedPage service.ResolvedPage
}

func (s *publicResolverServiceStub) ResolveBySlug(
	_ context.Context,
	scope coretenant.Scope,
	slug string,
	previewToken string,
) (service.ResolvedPage, error) {
	s.scope = scope
	s.slug = slug
	s.preview = previewToken
	return s.resolvedPage, nil
}

func (s *publicResolverServiceStub) ResolveByDomain(
	_ context.Context,
	scope coretenant.Scope,
	customDomain string,
	previewToken string,
) (service.ResolvedPage, error) {
	s.scope = scope
	s.customHost = customDomain
	s.preview = previewToken
	s.usedDomain = true
	return s.resolvedPage, nil
}

func TestPublicLandingResolveUsesTenantScopeFromMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	resolver := &publicResolverServiceStub{
		resolvedPage: service.ResolvedPage{
			Page: domain.LandingPage{
				ID:             "page-1",
				OrganizationID: "11111111-1111-1111-1111-111111111111",
				Slug:           "about",
				Status:         domain.PageStatusPublished,
			},
		},
	}
	handler := NewPublicLandingHandler(resolver, nil, nil, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
			OrganizationID:     "11111111-1111-1111-1111-111111111111",
			OrganizationSlug:   "budi",
			OrganizationType:   coretenant.OrganizationTypeCustomer,
			OrganizationStatus: coretenant.OrganizationStatusActive,
			ResolutionSource:   coretenant.ResolutionSourceCustomDomain,
			DataPlacement:      coretenant.DataPlacementShared,
			RequestHost:        "budi.app.zyad.test",
		})
		if err != nil {
			t.Fatalf("NewVerifiedContext() error = %v", err)
		}
		coremiddleware.SetTenantContext(c, tenantContext)
		c.Next()
	})
	handler.RegisterRoutes(router.Group(""))

	req := httptest.NewRequest(http.MethodGet, "/public/landing/resolve?slug=about", nil)
	req.Host = "budi.app.zyad.test"
	req.Header.Set("X-Forwarded-Host", "andi.app.zyad.test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if resolver.scope.OrganizationID() != "11111111-1111-1111-1111-111111111111" ||
		resolver.slug != "about" ||
		resolver.usedDomain {
		t.Fatalf("resolver call = scope:%q slug:%q usedDomain:%v",
			resolver.scope.OrganizationID(),
			resolver.slug,
			resolver.usedDomain,
		)
	}
}
