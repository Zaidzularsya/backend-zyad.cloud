package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

type formServiceStub struct {
	service.FormService
	calls int
}

func (s *formServiceStub) CreateForm(context.Context, coretenant.Scope, repository.CreateFormParams) (domain.LandingForm, error) {
	s.calls++
	return domain.LandingForm{ID: "f1"}, nil
}

func (s *formServiceStub) UpdateForm(context.Context, coretenant.Scope, string, repository.UpdateFormParams) (domain.LandingForm, error) {
	s.calls++
	return domain.LandingForm{ID: "f1"}, nil
}

func newFormRouter(t *testing.T, svc service.FormService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(tenantMiddleware(t, "11111111-1111-1111-1111-111111111111", coretenant.OrganizationTypeCustomer))
	h := NewAdminFormHandler(svc)
	router.POST("/pages/:id/forms", h.CreateForm)
	router.PATCH("/pages/:id/forms/:formId", h.UpdateForm)
	return router
}

func TestAdminForm_RedirectURLValidation(t *testing.T) {
	cases := []struct {
		url string
		ok  bool
	}{
		{"", true},
		{"/terima-kasih", true},
		{"https://example.com/x", true},
		{"http://example.com", true},
		{"javascript:alert(1)", false},
		{"data:text/html,x", false},
		{"//evil.com", false},
		{"ftp://x.com", false},
		{"example.com", false},
		{"/\\evil.com", false},
	}
	for _, tc := range cases {
		t.Run("create "+tc.url, func(t *testing.T) {
			svc := &formServiceStub{}
			body := `{"name":"F","key":"f","submit_label":"Kirim","redirect_url":"` + strings.ReplaceAll(tc.url, `\`, `\\`) + `"}`
			req := httptest.NewRequest(http.MethodPost, "/pages/p1/forms", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			newFormRouter(t, svc).ServeHTTP(rec, req)
			assertRedirectResult(t, rec, svc, tc.ok)
		})
		t.Run("update "+tc.url, func(t *testing.T) {
			svc := &formServiceStub{}
			body := `{"redirect_url":"` + strings.ReplaceAll(tc.url, `\`, `\\`) + `"}`
			req := httptest.NewRequest(http.MethodPatch, "/pages/p1/forms/f1", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			newFormRouter(t, svc).ServeHTTP(rec, req)
			assertRedirectResult(t, rec, svc, tc.ok)
		})
	}
}

func assertRedirectResult(t *testing.T, rec *httptest.ResponseRecorder, svc *formServiceStub, ok bool) {
	t.Helper()
	if ok {
		if rec.Code != http.StatusOK || svc.calls != 1 {
			t.Fatalf("status = %d calls = %d body=%s", rec.Code, svc.calls, rec.Body.String())
		}
		return
	}
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") || svc.calls != 0 {
		t.Fatalf("status = %d calls = %d body=%s", rec.Code, svc.calls, rec.Body.String())
	}
}
