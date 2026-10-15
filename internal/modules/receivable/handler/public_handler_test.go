package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/receivable/repository"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/service"
)

type fakePublicInvoices struct{ view service.PublicInvoiceView }

func (f *fakePublicInvoices) View(_ context.Context, token string) (service.PublicInvoiceView, error) {
	if token != "good" {
		return service.PublicInvoiceView{}, service.ErrLinkInvalid
	}
	return f.view, nil
}

func (f *fakePublicInvoices) PDF(_ context.Context, token string) (service.InvoicePDF, error) {
	if token != "good" {
		return service.InvoicePDF{}, service.ErrLinkInvalid
	}
	return service.InvoicePDF{Filename: "INV-1.pdf", Content: []byte("%PDF")}, nil
}

func date(s string) *time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return &d
}

func openView() service.PublicInvoiceView {
	return service.PublicInvoiceView{
		TenantName: "PT Zyad", State: service.PublicInvoiceStateOpen,
		Invoice: domain.Invoice{
			InvoiceNumber: "INV-1", Status: domain.InvoiceIssued, Currency: "IDR", GrandTotal: "333000.00", AmountPaid: "100000.00",
			IssueDate: date("2026-10-05"), DueDate: date("2026-10-12"), PeriodStart: date("2026-10-01"), PeriodEnd: date("2026-10-31"),
			Items: []domain.InvoiceItem{{Description: "Internet", Quantity: "1.00", Unit: "bulan", UnitPrice: "300000.00", LineTotal: "300000.00",
				PeriodStart: date("2026-10-01"), PeriodEnd: date("2026-10-31")}},
		},
	}
}

func publicInvoiceRouter(view service.PublicInvoiceView) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewPublicInvoiceHandler(&fakePublicInvoices{view: view}, nil).RegisterRoutes(r.Group("/api/v1"))
	return r
}

func get(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestPublicInvoiceViewPayload(t *testing.T) {
	w := get(publicInvoiceRouter(openView()), "/api/v1/public/invoices/good")
	for _, want := range []string{
		`"tenant_name":"PT Zyad"`, `"invoice_number":"INV-1"`, `"status":"issued"`, `"state":"open"`, `"due_date":"2026-10-12"`,
		`"period_start":"2026-10-01"`, `"grand_total":"333000.00"`, `"amount_paid":"100000.00"`, `"balance":"233000.00"`,
		`"can_pay":false`, `"items":[`, `"description":"Internet"`,
	} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("missing %s in %s", want, w.Body)
		}
	}
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Robots-Tag") != "noindex" {
		t.Fatalf("status=%d headers=%v", w.Code, w.Header())
	}
}

func TestPublicInvoiceVoidAndPaidStates(t *testing.T) {
	void := openView()
	void.State, void.Invoice.Status, void.CanPay = service.PublicInvoiceStateVoid, domain.InvoiceVoid, false
	if w := get(publicInvoiceRouter(void), "/api/v1/public/invoices/good"); !strings.Contains(w.Body.String(), `"state":"void"`) || !strings.Contains(w.Body.String(), `"can_pay":false`) {
		t.Fatalf("void = %s", w.Body)
	}
	paid := openView()
	paid.State, paid.Invoice.Status = service.PublicInvoiceStatePaid, domain.InvoicePaid
	if w := get(publicInvoiceRouter(paid), "/api/v1/public/invoices/good"); !strings.Contains(w.Body.String(), `"state":"paid"`) {
		t.Fatalf("paid = %s", w.Body)
	}
}

func TestPublicInvoiceCanPayIsPassedThrough(t *testing.T) {
	v := openView()
	v.CanPay = true
	if w := get(publicInvoiceRouter(v), "/api/v1/public/invoices/good"); !strings.Contains(w.Body.String(), `"can_pay":true`) {
		t.Fatalf("body = %s", w.Body)
	}
}

func TestPublicInvoiceUnknownTokenIsGeneric404WithoutToken(t *testing.T) {
	r := publicInvoiceRouter(openView())
	for _, path := range []string{"/api/v1/public/invoices/SECRET-TOKEN-123", "/api/v1/public/invoices/SECRET-TOKEN-123/pdf"} {
		w := get(r, path)
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "LINK_INVALID") || strings.Contains(w.Body.String(), "SECRET-TOKEN-123") {
			t.Fatalf("%s = %d %s", path, w.Code, w.Body)
		}
	}
}

func TestPublicInvoicePDFHeaders(t *testing.T) {
	w := get(publicInvoiceRouter(openView()), "/api/v1/public/invoices/good/pdf")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline") ||
		w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Robots-Tag") != "noindex" {
		t.Fatalf("pdf = %d %v", w.Code, w.Header())
	}
}

type fakePublicPayments struct {
	err       error
	checkouts int
}

func (f *fakePublicPayments) Checkout(_ context.Context, token string) (string, time.Time, error) {
	f.checkouts++
	if f.err != nil {
		return "", time.Time{}, f.err
	}
	if token != "good" {
		return "", time.Time{}, service.ErrLinkInvalid
	}
	return "https://doku.test/pay", time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC), nil
}

func (f *fakePublicPayments) SyncStatus(_ context.Context, token string) (domain.InvoiceStatus, error) {
	if token != "good" {
		return "", service.ErrLinkInvalid
	}
	return domain.InvoicePaid, nil
}

func paymentRouter(p *fakePublicPayments) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewPublicInvoiceHandler(&fakePublicInvoices{view: openView()}, nil).WithPayments(p).RegisterRoutes(r.Group("/api/v1"))
	return r
}

func post(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
	return w
}

func TestPublicInvoiceCheckout(t *testing.T) {
	p := &fakePublicPayments{}
	w := post(paymentRouter(p), "/api/v1/public/invoices/good/checkout")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"payment_url":"https://doku.test/pay"`) ||
		!strings.Contains(w.Body.String(), `"expires_at":"2026-10-05T04:00:00Z"`) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if w := post(paymentRouter(p), "/api/v1/public/invoices/bad/checkout"); w.Code != http.StatusNotFound {
		t.Fatalf("bad token status=%d", w.Code)
	}
}

func TestPublicInvoiceCheckoutErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{service.ErrOnlinePaymentUnavailable, http.StatusForbidden, "ONLINE_PAYMENT_UNAVAILABLE"},
		{repository.ErrInvoiceNotPayable, http.StatusConflict, "INVOICE_NOT_PAYABLE"},
		{coreerrors.New("PAYMENT_CHECKOUT_FAILED", "failed", http.StatusBadGateway), http.StatusBadGateway, "PAYMENT_CHECKOUT_FAILED"},
	}
	for _, tc := range cases {
		w := post(paymentRouter(&fakePublicPayments{err: tc.err}), "/api/v1/public/invoices/good/checkout")
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Errorf("%v: status=%d body=%s", tc.err, w.Code, w.Body)
		}
	}
	w := post(paymentRouter(&fakePublicPayments{err: service.ErrOnlinePaymentUnavailable}), "/api/v1/public/invoices/good/checkout")
	if !strings.Contains(w.Body.String(), "Pembayaran online belum tersedia untuk invoice ini.") {
		t.Fatalf("message: %s", w.Body)
	}
}

func TestPublicInvoiceStatus(t *testing.T) {
	w := get(paymentRouter(&fakePublicPayments{}), "/api/v1/public/invoices/good/status")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"paid"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if w := get(paymentRouter(&fakePublicPayments{}), "/api/v1/public/invoices/bad/status"); w.Code != http.StatusNotFound {
		t.Fatalf("bad token status=%d", w.Code)
	}
}
