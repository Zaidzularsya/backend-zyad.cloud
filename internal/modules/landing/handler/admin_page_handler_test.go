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
	if pageSvc.receivedCreate.Builder != domain.PageBuilderSections {
		t.Fatalf("expected builder to default to sections, got %q", pageSvc.receivedCreate.Builder)
	}
}

func TestAdminPageHandlerCreatePageAcceptsGrapesJSBuilder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pageSvc := &fakePageService{createPage: domain.LandingPage{ID: "p1", Builder: domain.PageBuilderGrapesJS}}
	handler := NewAdminPageHandler(pageSvc, nil, nil, nil)

	router := gin.New()
	router.POST("/admin/landing-pages", func(c *gin.Context) {
		tc := verifiedTenantContext(t)
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
		permissionmiddleware.SetUserID(c, "33333333-3333-3333-3333-333333333333")
		handler.CreatePage(c)
	})

	body := `{"name":"g","title":"g","slug":"g","page_type":"campaign","builder":"grapesjs","visibility":"public"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/landing-pages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body %s", rec.Code, rec.Body.String())
	}
	if pageSvc.receivedCreate.Builder != domain.PageBuilderGrapesJS {
		t.Fatalf("expected builder grapesjs, got %q", pageSvc.receivedCreate.Builder)
	}
	if !strings.Contains(rec.Body.String(), `"builder":"grapesjs"`) {
		t.Fatalf("response missing builder: %s", rec.Body.String())
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

func TestAdminPageHandlerUpdatePagePersistsSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pageSvc := &fakePageService{
		updatePage: domain.LandingPage{
			ID: "11111111-1111-1111-1111-111111111111",
			Settings: domain.PageSettings{
				FooterCopyrightText: "© 2026 Acme",
			},
		},
	}
	handler := NewAdminPageHandler(pageSvc, nil, nil, nil)

	router := gin.New()
	router.PATCH("/admin/landing-pages/:id", func(c *gin.Context) {
		tenantContext := verifiedTenantContext(t)
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tenantContext))
		permissionmiddleware.SetUserID(c, "33333333-3333-3333-3333-333333333333")
		handler.UpdatePage(c)
	})

	body := `{
		"settings": {
			"footer_copyright_text": "© 2026 Acme",
			"lead_notification_emails": ["ops@example.com"]
		}
	}`
	req := httptest.NewRequest(http.MethodPatch, "/admin/landing-pages/11111111-1111-1111-1111-111111111111", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body %s", recorder.Code, recorder.Body.String())
	}
	if pageSvc.receivedUpdate.Settings == nil {
		t.Fatal("expected Settings to be mapped into UpdatePageParams, got nil (regression of the Settings-drop bug)")
	}
	if pageSvc.receivedUpdate.Settings.FooterCopyrightText != "© 2026 Acme" {
		t.Fatalf("expected FooterCopyrightText to be mapped, got %q", pageSvc.receivedUpdate.Settings.FooterCopyrightText)
	}
	if len(pageSvc.receivedUpdate.Settings.LeadNotificationEmails) != 1 || pageSvc.receivedUpdate.Settings.LeadNotificationEmails[0] != "ops@example.com" {
		t.Fatalf("expected LeadNotificationEmails to be mapped, got %+v", pageSvc.receivedUpdate.Settings.LeadNotificationEmails)
	}
	if !strings.Contains(recorder.Body.String(), "Acme") {
		t.Fatalf("expected response body to reflect updated settings, got %s", recorder.Body.String())
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
	receivedUpdate repository.UpdatePageParams
	updatePage     domain.LandingPage
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
	_ context.Context,
	_ coretenant.Scope,
	_ string,
	params repository.UpdatePageParams,
) (domain.LandingPage, error) {
	s.receivedUpdate = params
	return s.updatePage, nil
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

func (s *fakePageService) InstantiateFromTemplate(
	context.Context,
	coretenant.Scope,
	service.InstantiatePageFromTemplateParams,
) (domain.LandingPage, error) {
	return domain.LandingPage{}, nil
}

func (s *fakePageService) Archive(context.Context, coretenant.Scope, string, string) error {
	return nil
}

func (s *fakePageService) Restore(context.Context, coretenant.Scope, string, string) error {
	return nil
}
