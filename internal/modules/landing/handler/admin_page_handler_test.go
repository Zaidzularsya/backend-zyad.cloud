package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	corevalidation "zyad.cloud/internal/core/validation"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

func TestAdminPageHandlerCreatePageAcceptsSlugValidator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	corevalidation.RegisterGinValidators()

	pageSvc := &fakePageService{
		createPage: domain.LandingPage{
			ID:               "11111111-1111-1111-1111-111111111111",
			OrganizationID:   "22222222-2222-2222-2222-222222222222",
			Name:             "tes",
			Title:            "tes",
			Slug:             "tes",
			Type:             domain.PageTypeCompanyProfile,
			Status:           domain.PageStatusDraft,
			Visibility:       domain.PageVisibilityPublic,
			Locale:           "id-ID",
			Timezone:         "Asia/Jakarta",
			PublishedVersion: 0,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
	}
	handler := NewAdminPageHandler(pageSvc, nil, nil, nil)

	router := gin.New()
	router.POST("/admin/landing-pages", func(c *gin.Context) {
		tenantContext := verifiedTenantContext(t)
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tenantContext))
		permissionmiddleware.SetUserID(c, "33333333-3333-3333-3333-333333333333")
		handler.CreatePage(c)
	})

	body := `{
		"name": "tes",
		"title": "tes",
		"slug": "tes",
		"page_type": "company_profile",
		"visibility": "public",
		"locale": "id-ID",
		"timezone": "Asia/Jakarta",
		"is_homepage": false
	}`
	req := httptest.NewRequest(http.MethodPost, "/admin/landing-pages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if pageSvc.receivedCreate.Slug != "tes" {
		t.Fatalf("expected slug tes, got %s", pageSvc.receivedCreate.Slug)
	}
	if pageSvc.receivedCreate.Type != domain.PageTypeCompanyProfile {
		t.Fatalf("expected page type company_profile, got %s", pageSvc.receivedCreate.Type)
	}
}

func TestAdminPageHandlerListDefaultsToNonTemplatePages(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pageSvc := &fakePageService{}
	handler := NewAdminPageHandler(pageSvc, nil, nil, nil)

	router := gin.New()
	router.GET("/admin/landing-pages", func(c *gin.Context) {
		tenantContext := verifiedTenantContext(t)
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tenantContext))
		handler.ListPages(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/landing-pages", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if pageSvc.receivedList.IsTemplate == nil {
		t.Fatal("expected list filter IsTemplate to default to false")
	}
	if *pageSvc.receivedList.IsTemplate {
		t.Fatal("expected list filter IsTemplate false for default page grid")
	}
}

func TestAdminPageHandlerListCanRequestTemplatePages(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pageSvc := &fakePageService{}
	handler := NewAdminPageHandler(pageSvc, nil, nil, nil)

	router := gin.New()
	router.GET("/admin/landing-pages", func(c *gin.Context) {
		tenantContext := verifiedTenantContext(t)
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tenantContext))
		handler.ListPages(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/landing-pages?is_template=true", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if pageSvc.receivedList.IsTemplate == nil {
		t.Fatal("expected list filter IsTemplate from query")
	}
	if !*pageSvc.receivedList.IsTemplate {
		t.Fatal("expected list filter IsTemplate true for template catalog")
	}
}

func verifiedTenantContext(t *testing.T) coretenant.Context {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     "22222222-2222-2222-2222-222222222222",
		OrganizationSlug:   "platform",
		OrganizationType:   coretenant.OrganizationTypePlatform,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceInternal,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("new tenant context: %v", err)
	}
	return tenantContext
}

type fakePageService struct {
	createPage     domain.LandingPage
	receivedCreate repository.CreatePageParams
	receivedList   repository.PageListFilter
}

func (s *fakePageService) Create(
	_ context.Context,
	_ coretenant.Scope,
	params repository.CreatePageParams,
) (domain.LandingPage, error) {
	s.receivedCreate = params
	return s.createPage, nil
}

func (s *fakePageService) Get(
	context.Context,
	coretenant.Scope,
	string,
) (domain.LandingPage, error) {
	return domain.LandingPage{}, nil
}

func (s *fakePageService) List(
	_ context.Context,
	_ coretenant.Scope,
	filter repository.PageListFilter,
) ([]domain.LandingPage, int64, error) {
	s.receivedList = filter
	return nil, 0, nil
}

func (s *fakePageService) Update(
	context.Context,
	coretenant.Scope,
	string,
	repository.UpdatePageParams,
) (domain.LandingPage, error) {
	return domain.LandingPage{}, nil
}

func (s *fakePageService) Delete(context.Context, coretenant.Scope, string) error {
	return nil
}

func (s *fakePageService) Duplicate(
	context.Context,
	coretenant.Scope,
	service.DuplicatePageParams,
) (domain.LandingPage, error) {
	return domain.LandingPage{}, nil
}

func (s *fakePageService) Archive(context.Context, coretenant.Scope, string, string) error {
	return nil
}

func (s *fakePageService) Restore(context.Context, coretenant.Scope, string, string) error {
	return nil
}
