package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/service"
)

type fakeActivationService struct {
	in     service.ActivationInput
	pageID string
	userID string
	orgID  string
	err    error
}

func (f *fakeActivationService) CreateInvoice(_ context.Context, scope coretenant.Scope, pageID string, in service.ActivationInput, userID string) (service.ActivationResult, error) {
	f.in, f.pageID, f.userID, f.orgID = in, pageID, userID, scope.OrganizationID()
	if f.err != nil {
		return service.ActivationResult{}, f.err
	}
	invoiceID := "inv-1"
	email := in.BuyerEmail
	return service.ActivationResult{
		Activation: domain.LandingPageActivation{
			ID: "act-1", PageID: pageID, InvoiceID: &invoiceID, BuyerName: in.BuyerName, BuyerContact: in.BuyerContact,
			BuyerEmail: &email, Price: "150000.00", DurationValue: in.DurationValue, DurationUnit: in.DurationUnit,
			PreviewTokenHash: "SECRET-HASH", InvoiceDueAt: time.Date(2026, 10, 23, 11, 0, 0, 0, time.UTC),
			Status: domain.ActivationStatusOpen,
		},
		PreviewURL: "https://r.example.test/preview/RAWTOKEN",
		PaymentURL: "https://app.example.test/i/link",
	}, nil
}

func serveActivation(t *testing.T, svc service.ActivationService, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewActivationHandler(svc)
	router := gin.New()
	router.POST("/app/landing/pages/:id/activation-invoice", func(c *gin.Context) {
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), verifiedTenantContext(t)))
		c.Set(permissionmiddleware.UserIDContextKey, "user-1")
		h.CreateInvoice(c)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/app/landing/pages/page-a/activation-invoice", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func TestActivationHandlerCreatesInvoice(t *testing.T) {
	svc := &fakeActivationService{}
	rec := serveActivation(t, svc, `{"buyer_name":"Budi","buyer_contact":"0812","buyer_email":"b@example.test",
		"price":"150000","duration_value":2,"duration_unit":"week","invoice_due_days":3}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if svc.pageID != "page-a" || svc.userID != "user-1" || svc.orgID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("service called with page=%s user=%s org=%s", svc.pageID, svc.userID, svc.orgID)
	}
	in := svc.in
	if in.BuyerName != "Budi" || in.BuyerContact != "0812" || in.BuyerEmail != "b@example.test" || in.Price != "150000" ||
		in.DurationValue != 2 || in.DurationUnit != domain.ActivationDurationWeek ||
		in.InvoiceDueHours != nil || in.InvoiceDueDays == nil || *in.InvoiceDueDays != 3 {
		t.Fatalf("input = %+v", in)
	}
	body := rec.Body.String()
	if strings.Contains(body, "SECRET-HASH") {
		t.Fatal("response must not expose the token hash")
	}
	var resp struct {
		Data struct {
			Activation struct {
				ID, Status, InvoiceID, Price string
			} `json:"activation"`
			PreviewURL string `json:"preview_url"`
			PaymentURL string `json:"payment_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.PreviewURL == "" || resp.Data.PaymentURL != "https://app.example.test/i/link" || !strings.Contains(body, `"invoice_id":"inv-1"`) ||
		!strings.Contains(body, `"status":"open"`) || !strings.Contains(body, `"price":"150000.00"`) {
		t.Fatalf("response = %s", body)
	}
}

func TestActivationHandlerRejectsBadJSON(t *testing.T) {
	for _, body := range []string{`{`, `{"price":150000}`, `{"duration_value":"x"}`} {
		svc := &fakeActivationService{}
		rec := serveActivation(t, svc, body)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: status = %d", body, rec.Code)
		}
		if svc.pageID != "" {
			t.Fatalf("%s: service must not be called", body)
		}
	}
}

func TestActivationHandlerMapsServiceErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{
		{coreerrors.New("FEATURE_NOT_ENABLED", "x", http.StatusForbidden), http.StatusForbidden},
		{coreerrors.New("PAGE_NOT_FOUND", "x", http.StatusNotFound), http.StatusNotFound},
		{coreerrors.New("ACTIVATION_OPEN", "x", http.StatusConflict), http.StatusConflict},
		{coreerrors.New("VALIDATION_ERROR", "x", http.StatusUnprocessableEntity), http.StatusUnprocessableEntity},
	} {
		rec := serveActivation(t, &fakeActivationService{err: tc.err}, `{"buyer_name":"a","buyer_contact":"b","price":"1","duration_value":1,"duration_unit":"day"}`)
		if rec.Code != tc.code {
			t.Fatalf("%v: status = %d", tc.err, rec.Code)
		}
	}
}
