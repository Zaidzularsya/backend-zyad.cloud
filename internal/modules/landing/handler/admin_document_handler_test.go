package handler

import (
	"context"
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

type fakeDocumentService struct {
	getResult  domain.LandingPageDocument
	getErr     error
	saveResult domain.LandingPageDocument
	saveErr    error
	saveParams service.SaveDocumentParams
	getPageID  string
	saveCalled bool
	getCalled  bool
}

func (s *fakeDocumentService) Get(_ context.Context, _ coretenant.Scope, pageID string) (domain.LandingPageDocument, error) {
	s.getCalled = true
	s.getPageID = pageID
	return s.getResult, s.getErr
}

func (s *fakeDocumentService) Save(_ context.Context, _ coretenant.Scope, params service.SaveDocumentParams) (domain.LandingPageDocument, error) {
	s.saveCalled = true
	s.saveParams = params
	return s.saveResult, s.saveErr
}

const testDocumentPageID = "11111111-1111-1111-1111-111111111111"

func documentRouter(t *testing.T, method, path string, fn func(*gin.Context)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	corevalidation.RegisterGinValidators()

	router := gin.New()
	router.Handle(method, path, func(c *gin.Context) {
		tc := verifiedTenantContext(t)
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
		permissionmiddleware.SetUserID(c, "33333333-3333-3333-3333-333333333333")
		fn(c)
	})
	return router
}

func TestAdminDocumentHandlerGetReturnsDocument(t *testing.T) {
	svc := &fakeDocumentService{getResult: domain.LandingPageDocument{
		LandingPageID: testDocumentPageID,
		Project:       map[string]any{"pages": []any{}},
		HTML:          "<p>hi</p>",
		CSS:           "p{color:red}",
	}}
	h := NewAdminDocumentHandler(svc)
	router := documentRouter(t, http.MethodGet, "/admin/landing-pages/:id/document", h.GetDocument)

	req := httptest.NewRequest(http.MethodGet, "/admin/landing-pages/"+testDocumentPageID+"/document", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	if !svc.getCalled || svc.getPageID != testDocumentPageID {
		t.Fatalf("Get not called with page id, got %q", svc.getPageID)
	}
	// Gin's JSON encoder escapes '<' → <.
	if !strings.Contains(rec.Body.String(), `\u003cp\u003ehi`) ||
		!strings.Contains(rec.Body.String(), `"css":"p{color:red}"`) {
		t.Fatalf("body missing html/css: %s", rec.Body.String())
	}
}

func TestAdminDocumentHandlerSavePassesParamsAndActor(t *testing.T) {
	svc := &fakeDocumentService{saveResult: domain.LandingPageDocument{LandingPageID: testDocumentPageID}}
	h := NewAdminDocumentHandler(svc)
	router := documentRouter(t, http.MethodPut, "/admin/landing-pages/:id/document", h.SaveDocument)

	body := `{"project":{"pages":[]},"html":"<section></section>","css":"body{margin:0}"}`
	req := httptest.NewRequest(http.MethodPut, "/admin/landing-pages/"+testDocumentPageID+"/document", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body %s", rec.Code, rec.Body.String())
	}
	if !svc.saveCalled {
		t.Fatal("Save not called")
	}
	if svc.saveParams.PageID != testDocumentPageID ||
		svc.saveParams.HTML != "<section></section>" ||
		svc.saveParams.CSS != "body{margin:0}" ||
		svc.saveParams.ActorID != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("params = %#v", svc.saveParams)
	}
}

func TestAdminDocumentHandlerSaveRejectsInvalidJSON(t *testing.T) {
	svc := &fakeDocumentService{}
	h := NewAdminDocumentHandler(svc)
	router := documentRouter(t, http.MethodPut, "/admin/landing-pages/:id/document", h.SaveDocument)

	req := httptest.NewRequest(http.MethodPut, "/admin/landing-pages/"+testDocumentPageID+"/document", strings.NewReader("{ not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
	if svc.saveCalled {
		t.Fatal("Save should not be called on bad JSON")
	}
}
