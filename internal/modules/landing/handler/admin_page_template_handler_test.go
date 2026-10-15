package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	corevalidation "zyad.cloud/internal/core/validation"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/service"
)

type fakePageTemplateService struct {
	viewer     service.PageTemplateViewer
	err        error
	items      []domain.PageTemplate
	got        domain.PageTemplate
	metaInput  service.UpdatePageTemplateMetaInput
	createCall bool
}

func (f *fakePageTemplateService) ListPublished(_ context.Context, v service.PageTemplateViewer) ([]domain.PageTemplate, error) {
	f.viewer = v
	return f.items, f.err
}
func (f *fakePageTemplateService) Get(_ context.Context, v service.PageTemplateViewer, _ string) (domain.PageTemplate, error) {
	f.viewer = v
	return f.got, f.err
}
func (f *fakePageTemplateService) ListAll(_ context.Context, v service.PageTemplateViewer) ([]domain.PageTemplate, error) {
	f.viewer = v
	return f.items, f.err
}
func (f *fakePageTemplateService) CreateFromPage(_ context.Context, v service.PageTemplateViewer, _ service.CreatePageTemplateInput) (domain.PageTemplate, error) {
	f.viewer = v
	f.createCall = true
	return f.got, f.err
}
func (f *fakePageTemplateService) RefreshFromPage(context.Context, service.PageTemplateViewer, string) (domain.PageTemplate, error) {
	return f.got, f.err
}
func (f *fakePageTemplateService) CopyToPage(context.Context, service.PageTemplateViewer, string) (domain.LandingPage, error) {
	return domain.LandingPage{ID: "p", Slug: "template-x"}, f.err
}
func (f *fakePageTemplateService) UpdateMeta(_ context.Context, _ service.PageTemplateViewer, _ string, in service.UpdatePageTemplateMetaInput) (domain.PageTemplate, error) {
	f.metaInput = in
	return f.got, f.err
}

func templateRouter(t *testing.T, orgType coretenant.OrganizationType, h *AdminPageTemplateHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	corevalidation.RegisterGinValidators()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
			OrganizationID: "22222222-2222-2222-2222-222222222222", OrganizationSlug: "acme",
			OrganizationType: orgType, OrganizationStatus: coretenant.OrganizationStatusActive,
			ResolutionSource: coretenant.ResolutionSourceInternal, DataPlacement: coretenant.DataPlacementShared,
		})
		if err != nil {
			t.Fatalf("tenant context: %v", err)
		}
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
		permissionmiddleware.SetUserID(c, "33333333-3333-3333-3333-333333333333")
		c.Next()
	})
	g := router.Group("/admin/landing-templates")
	g.GET("", h.List)
	g.GET("/all", h.ListAll)
	g.GET("/:id", h.Get)
	g.POST("", h.Create)
	g.POST("/:id/copy-to-page", h.CopyToPage)
	g.PATCH("/:id", h.Patch)
	return router
}

func doTemplateRequest(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestPageTemplateHandler_GetHiddenIs404(t *testing.T) {
	svc := &fakePageTemplateService{err: service.ErrPageTemplateNotFound}
	r := templateRouter(t, coretenant.OrganizationTypeCustomer, NewAdminPageTemplateHandler(svc))
	rec := doTemplateRequest(r, http.MethodGet, "/admin/landing-templates/abc", "")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "TEMPLATE_NOT_FOUND") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestPageTemplateHandler_ViewerFromContext(t *testing.T) {
	for _, tc := range []struct {
		org      coretenant.OrganizationType
		platform bool
	}{{coretenant.OrganizationTypeCustomer, false}, {coretenant.OrganizationTypePlatform, true}} {
		svc := &fakePageTemplateService{}
		r := templateRouter(t, tc.org, NewAdminPageTemplateHandler(svc))
		if rec := doTemplateRequest(r, http.MethodGet, "/admin/landing-templates", ""); rec.Code != http.StatusOK {
			t.Fatalf("status %d %s", rec.Code, rec.Body.String())
		}
		if svc.viewer.IsPlatform != tc.platform || svc.viewer.ActorID == "" {
			t.Errorf("org=%s viewer=%+v", tc.org, svc.viewer)
		}
	}
}

func TestPageTemplateHandler_CreateValidation(t *testing.T) {
	svc := &fakePageTemplateService{}
	r := templateRouter(t, coretenant.OrganizationTypePlatform, NewAdminPageTemplateHandler(svc))
	rec := doTemplateRequest(r, http.MethodPost, "/admin/landing-templates", `{"name":"x","slug":"x","category":"dasar"}`)
	if rec.Code != http.StatusUnprocessableEntity || svc.createCall {
		t.Fatalf("got %d createCalled=%v", rec.Code, svc.createCall)
	}
}

func TestPageTemplateHandler_CreateMapsSlugTaken(t *testing.T) {
	svc := &fakePageTemplateService{err: service.ErrPageTemplateSlugTaken}
	r := templateRouter(t, coretenant.OrganizationTypePlatform, NewAdminPageTemplateHandler(svc))
	rec := doTemplateRequest(r, http.MethodPost, "/admin/landing-templates",
		`{"page_id":"p","name":"x","slug":"x","category":"dasar"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "TEMPLATE_SLUG_TAKEN") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestPageTemplateHandler_ListShape(t *testing.T) {
	svc := &fakePageTemplateService{items: []domain.PageTemplate{{ID: "1", Slug: "a", HTML: "<p>x</p>"}}}
	r := templateRouter(t, coretenant.OrganizationTypeCustomer, NewAdminPageTemplateHandler(svc))
	rec := doTemplateRequest(r, http.MethodGet, "/admin/landing-templates", "")
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Data) != 1 {
		t.Fatalf("decode: %v %s", err, rec.Body.String())
	}
	for _, key := range []string{"html", "css", "status", "source"} {
		if _, ok := body.Data[0][key]; ok {
			t.Errorf("list item must not contain %q", key)
		}
	}
	if _, ok := body.Data[0]["platform_only"]; !ok {
		t.Error("list item must contain platform_only")
	}
}

func TestPageTemplateHandler_PatchPartial(t *testing.T) {
	svc := &fakePageTemplateService{}
	r := templateRouter(t, coretenant.OrganizationTypePlatform, NewAdminPageTemplateHandler(svc))
	rec := doTemplateRequest(r, http.MethodPatch, "/admin/landing-templates/abc", `{"status":"archived"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	in := svc.metaInput
	if in.Status == nil || *in.Status != domain.PageTemplateStatusArchived {
		t.Errorf("status: %+v", in.Status)
	}
	if in.Name != nil || in.Description != nil || in.Category != nil || in.SortOrder != nil || in.PlatformOnly != nil {
		t.Errorf("other fields must be nil: %+v", in)
	}
}

func TestPageTemplateHandler_AllRouteNotShadowedByID(t *testing.T) {
	svc := &fakePageTemplateService{items: []domain.PageTemplate{{ID: "1", Slug: "a", Status: domain.PageTemplateStatusDraft}}}
	r := templateRouter(t, coretenant.OrganizationTypePlatform, NewAdminPageTemplateHandler(svc))
	rec := doTemplateRequest(r, http.MethodGet, "/admin/landing-templates/all", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"draft"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}
