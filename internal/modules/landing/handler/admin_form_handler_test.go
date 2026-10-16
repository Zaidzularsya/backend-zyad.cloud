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

// flexFormServiceStub merekam parameter yang diterima handler dan bisa mengembalikan error terprogram.
type flexFormServiceStub struct {
	service.FormService
	createParams  repository.CreateFormParams
	replaceParams []repository.CreateFormFieldParams
	replaceErr    error
	updateCalls   int
}

func (s *flexFormServiceStub) CreateForm(_ context.Context, _ coretenant.Scope, p repository.CreateFormParams) (domain.LandingForm, error) {
	s.createParams = p
	return domain.LandingForm{ID: "f1", AfterSubmit: p.AfterSubmit, WhatsAppNumber: p.WhatsAppNumber}, nil
}

func (s *flexFormServiceStub) UpdateForm(context.Context, coretenant.Scope, string, repository.UpdateFormParams) (domain.LandingForm, error) {
	s.updateCalls++
	return domain.LandingForm{ID: "f1"}, nil
}

func (s *flexFormServiceStub) ReplaceFields(_ context.Context, _ coretenant.Scope, _ string, p []repository.CreateFormFieldParams) ([]domain.LandingFormField, error) {
	s.replaceParams = p
	return nil, s.replaceErr
}

func newFlexFormRouter(t *testing.T, svc service.FormService) *gin.Engine {
	t.Helper()
	router := newFormRouter(t, svc)
	router.PUT("/pages/:id/forms/:formId/fields", NewAdminFormHandler(svc).ReplaceFields)
	return router
}

func doJSON(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAdminCreateForm_AcceptsAfterSubmitWhatsApp(t *testing.T) {
	svc := &flexFormServiceStub{}
	rec := doJSON(newFlexFormRouter(t, svc), http.MethodPost, "/pages/p1/forms",
		`{"name":"F","key":"f","submit_label":"Kirim","after_submit":"whatsapp","whatsapp_number":"0812 3456 7890","whatsapp_message":"Halo {nama}"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if svc.createParams.AfterSubmit != domain.AfterSubmitWhatsApp || svc.createParams.WhatsAppNumber != "0812 3456 7890" || svc.createParams.WhatsAppMessage != "Halo {nama}" {
		t.Fatalf("params not forwarded: %+v", svc.createParams)
	}
}

func TestAdminUpdateForm_RejectsUnknownAfterSubmit(t *testing.T) {
	svc := &flexFormServiceStub{}
	rec := doJSON(newFlexFormRouter(t, svc), http.MethodPatch, "/pages/p1/forms/f1", `{"after_submit":"popup"}`)
	if rec.Code != http.StatusUnprocessableEntity || svc.updateCalls != 0 {
		t.Fatalf("status = %d calls = %d body=%s", rec.Code, svc.updateCalls, rec.Body.String())
	}
}

func TestAdminReplaceFields_CoreFields422(t *testing.T) {
	svc := &flexFormServiceStub{replaceErr: service.ErrCoreFieldsRequired}
	rec := doJSON(newFlexFormRouter(t, svc), http.MethodPut, "/pages/p1/forms/f1/fields",
		`{"fields":[{"key":"name","type":"text","label":"Nama","required":true,"sort_order":0}]}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "LANDING_FORM_CORE_FIELDS") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminReplaceFields_PersistsDefaultValue(t *testing.T) {
	svc := &flexFormServiceStub{}
	rec := doJSON(newFlexFormRouter(t, svc), http.MethodPut, "/pages/p1/forms/f1/fields",
		`{"fields":[{"key":"interest","type":"hidden","label":"Minat","required":false,"sort_order":0,"default_value":"Paket 20 Mbps"}]}`)
	if rec.Code != http.StatusOK || len(svc.replaceParams) != 1 || svc.replaceParams[0].DefaultValue != "Paket 20 Mbps" {
		t.Fatalf("status = %d params=%+v body=%s", rec.Code, svc.replaceParams, rec.Body.String())
	}
}

func TestAdminReplaceFields_InvalidDefault422(t *testing.T) {
	svc := &flexFormServiceStub{replaceErr: service.ErrInvalidDefaultValue}
	rec := doJSON(newFlexFormRouter(t, svc), http.MethodPut, "/pages/p1/forms/f1/fields",
		`{"fields":[{"key":"interest","type":"hidden","label":"Minat","sort_order":0,"default_value":"x"}]}`)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
