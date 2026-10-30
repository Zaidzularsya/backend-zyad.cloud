package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

func TestPublicPreviewActivationTokenServesPageWithNoindex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := &publicResolverServiceStub{
		activationFn: func(string) (service.ResolvedPage, error) {
			return service.ResolvedPage{Page: domain.LandingPage{ID: "page-a", Slug: "undangan-budi"}, IsDraft: true}, nil
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/public/landing/preview/tok-a", nil)
	rec := httptest.NewRecorder()
	renderRouter(t, resolver).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if robots := rec.Header().Get("X-Robots-Tag"); !strings.Contains(robots, "noindex") {
		t.Fatalf("X-Robots-Tag = %q, want noindex", robots)
	}
	if resolver.activationToken != "tok-a" || resolver.scope.OrganizationID() != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("activation lookup token=%q org=%q", resolver.activationToken, resolver.scope.OrganizationID())
	}
	if !strings.Contains(rec.Body.String(), "page-a") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPublicPreviewUnknownTokenWithoutSlugIs404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := &publicResolverServiceStub{
		resolvedPage: service.ResolvedPage{Page: domain.LandingPage{ID: "home", Status: domain.PageStatusPublished}},
	}
	req := httptest.NewRequest(http.MethodGet, "/public/landing/preview/tidak-dikenal", nil)
	req.Host = "budi.app.zyad.test"
	rec := httptest.NewRecorder()
	renderRouter(t, resolver).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if resolver.usedDomain || resolver.slug != "" {
		t.Fatal("an unknown token must not fall back to the host's page")
	}
}

func TestPublicPreviewClosedActivationIs404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := &publicResolverServiceStub{
		activationFn: func(string) (service.ResolvedPage, error) { return service.ResolvedPage{}, service.ErrPageNotFound },
	}
	req := httptest.NewRequest(http.MethodGet, "/public/landing/preview/tok-due?slug=about", nil)
	rec := httptest.NewRecorder()
	renderRouter(t, resolver).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if resolver.slug != "" {
		t.Fatal("a closed activation token must not fall back to the slug preview")
	}
}

func TestPublicPreviewActivationTokenOfOtherPageDoesNotOpenSlug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := &publicResolverServiceStub{
		activationFn: func(string) (service.ResolvedPage, error) {
			return service.ResolvedPage{Page: domain.LandingPage{ID: "page-b", Slug: "lain"}}, nil
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/public/landing/preview/tok-b?slug=undangan-budi", nil)
	rec := httptest.NewRecorder()
	renderRouter(t, resolver).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

// --- real resolver with stub repositories: public delivery honours active_until ---

type expiryResolverRepo struct {
	repository.ResolverRepository
	page domain.LandingPage
}

func (r *expiryResolverRepo) ResolveBySlug(context.Context, coretenant.Scope, string) (domain.LandingPage, error) {
	return r.page, nil
}
func (r *expiryResolverRepo) ResolveByDomain(context.Context, coretenant.Scope, string) (domain.LandingPage, error) {
	return r.page, nil
}
func (r *expiryResolverRepo) ResolveBranding(context.Context, coretenant.Scope, string) (domain.LandingBranding, error) {
	return domain.LandingBranding{ID: "b1"}, nil
}
func (r *expiryResolverRepo) ResolveForms(context.Context, coretenant.Scope, string) ([]domain.LandingForm, error) {
	return nil, nil
}
func (r *expiryResolverRepo) ResolveMenus(context.Context, coretenant.Scope) ([]domain.LandingMenu, error) {
	return nil, nil
}
func (r *expiryResolverRepo) ResolveVersions(context.Context, coretenant.Scope, string) ([]domain.LandingPageVersion, error) {
	return []domain.LandingPageVersion{{Version: 1, Snapshot: map[string]any{
		"html": "<main><h1>Undangan Budi</h1></main>", "css": "",
	}}}, nil
}

type noDocumentRepo struct{ repository.DocumentRepository }

func (noDocumentRepo) GetByPageID(context.Context, coretenant.Scope, string) (domain.LandingPageDocument, error) {
	return domain.LandingPageDocument{}, pgx.ErrNoRows
}

func expiryRouter(t *testing.T, activeUntil *time.Time) *gin.Engine {
	t.Helper()
	page := domain.LandingPage{
		ID: "page-a", OrganizationID: "11111111-1111-1111-1111-111111111111", Slug: "undangan-budi",
		Title: "Undangan", Builder: domain.PageBuilderGrapesJS, Status: domain.PageStatusPublished,
		Visibility: domain.PageVisibilityPublic, ActiveUntil: activeUntil,
	}
	resolver := service.NewResolverService(nil, &expiryResolverRepo{page: page}, nil, nil, nil, nil, nil, nil, noDocumentRepo{}, nil)
	router := gin.New()
	router.Use(tenantMiddleware(t, "11111111-1111-1111-1111-111111111111", coretenant.OrganizationTypeCustomer))
	NewPublicLandingHandler(resolver, nil, nil, nil, nil).RegisterRoutes(router.Group(""))
	return router
}

func TestPublicDeliveryHidesPageWhoseActiveUntilPassed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	past := time.Now().UTC().Add(-time.Hour)
	router := expiryRouter(t, &past)

	for _, path := range []string{
		"/public/landing/resolve?slug=undangan-budi",
		"/public/landing/render/undangan-budi",
		"/public/landing/render?slug=undangan-budi",
		"/public/landing/render",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404 body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestPublicDeliveryServesPageWithNullOrFutureActiveUntil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	future := time.Now().UTC().Add(time.Hour)
	for name, activeUntil := range map[string]*time.Time{"null (regression)": nil, "future": &future} {
		router := expiryRouter(t, activeUntil)
		for _, path := range []string{
			"/public/landing/resolve?slug=undangan-budi",
			"/public/landing/render/undangan-budi",
		} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s %s: status = %d body=%s", name, path, rec.Code, rec.Body.String())
			}
		}
	}
}
