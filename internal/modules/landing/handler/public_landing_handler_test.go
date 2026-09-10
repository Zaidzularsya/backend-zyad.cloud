package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func renderRouter(t *testing.T, resolver *publicResolverServiceStub) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
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
		coremiddleware.SetTenantContext(c, tc)
		c.Next()
	})
	NewPublicLandingHandler(resolver, nil, nil, nil).RegisterRoutes(router.Group(""))
	return router
}

func TestPublicLandingRenderHTMLServesGrapesDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)

	resolver := &publicResolverServiceStub{
		resolvedPage: service.ResolvedPage{
			Builder: string(domain.PageBuilderGrapesJS),
			Page: domain.LandingPage{
				Slug:       "promo",
				Title:      "Promo",
				Status:     domain.PageStatusPublished,
				Visibility: domain.PageVisibilityPublic,
				SEO:        map[string]any{"meta_title": "Promo Hebat"},
			},
			HTML: "<main><h1>Promo</h1></main>",
			CSS:  "h1{color:red}",
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/public/landing/render/promo", nil)
	req.Host = "budi.app.zyad.test"
	rec := httptest.NewRecorder()
	renderRouter(t, resolver).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q", ct)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'none'") {
		t.Fatalf("missing CSP script-src none: %q", csp)
	}
	if resolver.slug != "promo" {
		t.Fatalf("resolver slug = %q", resolver.slug)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<title>Promo Hebat</title>") || !strings.Contains(body, "<h1>Promo</h1>") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestPublicLandingRenderHTMLNotFoundForSectionsPage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	resolver := &publicResolverServiceStub{
		resolvedPage: service.ResolvedPage{
			Page: domain.LandingPage{Slug: "about", Status: domain.PageStatusPublished},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/public/landing/render?slug=about", nil)
	req.Host = "budi.app.zyad.test"
	rec := httptest.NewRecorder()
	renderRouter(t, resolver).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a non-GrapesJS page, got %d body=%s", rec.Code, rec.Body.String())
	}
}
