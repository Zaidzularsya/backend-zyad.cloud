package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/service"
)

type fakePublicQuotations struct {
	lastApprove service.PublicResponseInput
	revisionErr error
}

func (f *fakePublicQuotations) view(state service.PublicQuotationState) service.PublicQuotationView {
	return service.PublicQuotationView{
		TenantName: "PT Zyad", QuotationNumber: "QUO-1", Status: domain.QuotationStatusApproved, State: state,
		Quotation: domain.Quotation{Currency: "IDR", GrandTotal: "100.00", OneTimeTotal: "100.00", FirstInvoiceTotal: "100.00",
			Items: []domain.QuotationItem{{Description: "A", Quantity: "1.00", UnitPrice: "100.00", LineTotal: "100.00"}}},
	}
}

func (f *fakePublicQuotations) View(_ context.Context, token string) (service.PublicQuotationView, error) {
	if token != "good" {
		return service.PublicQuotationView{}, service.ErrLinkInvalid
	}
	return f.view(service.PublicQuotationStateActive), nil
}

func (f *fakePublicQuotations) PDF(_ context.Context, token string) (service.QuotationPDF, error) {
	if token != "good" {
		return service.QuotationPDF{}, service.ErrLinkInvalid
	}
	return service.QuotationPDF{Filename: "QUO-1.pdf", Content: []byte("%PDF")}, nil
}

func (f *fakePublicQuotations) Approve(_ context.Context, token string, in service.PublicResponseInput) (service.PublicQuotationView, error) {
	if token != "good" {
		return service.PublicQuotationView{}, service.ErrLinkInvalid
	}
	f.lastApprove = in
	return f.view(service.PublicQuotationStateDecided), nil
}

func (f *fakePublicQuotations) RequestRevision(_ context.Context, token string, in service.PublicResponseInput) (service.PublicQuotationView, error) {
	if f.revisionErr != nil {
		return service.PublicQuotationView{}, f.revisionErr
	}
	if token != "good" {
		return service.PublicQuotationView{}, service.ErrLinkInvalid
	}
	if len(in.Categories) == 0 {
		return service.PublicQuotationView{}, service.ErrInvalidResponse
	}
	return f.view(service.PublicQuotationStateDecided), nil
}

func publicRouter(svc *fakePublicQuotations) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewPublicQuotationHandler(svc, nil).RegisterRoutes(r.Group("/api/v1"))
	return r
}

func TestPublicQuotationApproveRequiresAgree(t *testing.T) {
	r := publicRouter(&fakePublicQuotations{})
	if w := performJSON(r, http.MethodPost, "/api/v1/public/quotations/good/approve", `{"responder_name":"Budi"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("without agree = %d %s", w.Code, w.Body)
	}
	svc := &fakePublicQuotations{}
	r = publicRouter(svc)
	w := performJSON(r, http.MethodPost, "/api/v1/public/quotations/good/approve", `{"responder_name":"Budi","agree":true}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"state":"decided"`) || svc.lastApprove.ResponderName != "Budi" {
		t.Fatalf("approve = %d %s", w.Code, w.Body)
	}
}

func TestPublicQuotationUnknownTokenIsGeneric404WithoutToken(t *testing.T) {
	r := publicRouter(&fakePublicQuotations{})
	w := performJSON(r, http.MethodGet, "/api/v1/public/quotations/SECRET-TOKEN-123", "")
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "LINK_INVALID") || strings.Contains(w.Body.String(), "SECRET-TOKEN-123") {
		t.Fatalf("unknown = %d %s", w.Code, w.Body)
	}
}

func TestPublicQuotationRevisionValidationAndConflict(t *testing.T) {
	r := publicRouter(&fakePublicQuotations{})
	if w := performJSON(r, http.MethodPost, "/api/v1/public/quotations/good/revision", `{"responder_name":"Budi","categories":[]}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("service validation = %d %s", w.Code, w.Body)
	}
	r = publicRouter(&fakePublicQuotations{revisionErr: service.ErrQuotationNotRespondable})
	w := performJSON(r, http.MethodPost, "/api/v1/public/quotations/good/revision", `{"responder_name":"Budi","categories":["price"]}`)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "QUOTATION_NOT_RESPONDABLE") {
		t.Fatalf("conflict = %d %s", w.Code, w.Body)
	}
}

func TestPublicQuotationViewPayloadAndPDFHeaders(t *testing.T) {
	r := publicRouter(&fakePublicQuotations{})
	w := performJSON(r, http.MethodGet, "/api/v1/public/quotations/good", "")
	for _, want := range []string{`"tenant_name":"PT Zyad"`, `"state":"active"`, `"grand_total":"100.00"`, `"items":[`, `"last_response":null`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("missing %s in %s", want, w.Body)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/quotations/good/pdf", nil)
	pw := httptest.NewRecorder()
	r.ServeHTTP(pw, req)
	if pw.Code != http.StatusOK || !strings.HasPrefix(pw.Header().Get("Content-Disposition"), "inline") ||
		pw.Header().Get("Cache-Control") != "no-store" || pw.Header().Get("X-Robots-Tag") != "noindex" {
		t.Fatalf("pdf = %d %v", pw.Code, pw.Header())
	}
}
