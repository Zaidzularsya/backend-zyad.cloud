package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/platform/doku"
)

const (
	dokuTestClientID  = "BRN-0001-TEST"
	dokuTestSecretKey = "SK-test-secret"
	dokuWebhookPath   = "/api/v1/webhooks/doku"
)

type dokuProcessorStub struct {
	got       []service.DokuNotification
	processed bool
	err       error
}

func (s *dokuProcessorStub) HandleNotification(_ context.Context, n service.DokuNotification) (bool, error) {
	s.got = append(s.got, n)
	return s.processed, s.err
}

func newDokuWebhookTestRouter(p DokuProcessor, clientID, secretKey string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewDokuWebhookHandler(p, clientID, secretKey, nil).RegisterRoutes(router.Group("/api/v1"))
	return router
}

func signedDokuRequest(body []byte, secretKey string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, dokuWebhookPath, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Client-Id", dokuTestClientID)
	request.Header.Set("Request-Id", "notify-1")
	request.Header.Set("Request-Timestamp", "2026-07-09T01:02:03Z")
	request.Header.Set("Digest", doku.Digest(body))
	request.Header.Set("Signature", doku.Signature(
		dokuTestClientID, "notify-1", "2026-07-09T01:02:03Z", dokuWebhookPath, doku.Digest(body), secretKey,
	))
	return request
}

func dokuBody(invoice, status string) []byte {
	return []byte(`{
	"channel": {"id": "VIRTUAL_ACCOUNT_BCA"},
	"order": {"invoice_number": "` + invoice + `", "amount": 199000},
	"transaction": {"status": "` + status + `", "date": "2026-07-09T01:02:00Z", "original_request_id": "req-1"}
}`)
}

func TestDokuWebhookDispatchesReceivableInvoice(t *testing.T) {
	p := &dokuProcessorStub{processed: true}
	router := newDokuWebhookTestRouter(p, dokuTestClientID, dokuTestSecretKey)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, signedDokuRequest(dokuBody("RCV-2026-0001", "SUCCESS"), dokuTestSecretKey))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(p.got) != 1 {
		t.Fatalf("processor calls = %d, want 1", len(p.got))
	}
	n := p.got[0]
	if n.InvoiceNumber != "RCV-2026-0001" || n.Status != "SUCCESS" || n.Amount != "199000" ||
		n.PaymentMethod != "VIRTUAL_ACCOUNT_BCA" || n.ProviderReference != "req-1" {
		t.Fatalf("notification = %#v", n)
	}
	if n.PaidAt == nil || n.PaidAt.UTC().Format("2006-01-02T15:04:05Z") != "2026-07-09T01:02:00Z" {
		t.Fatalf("paid at = %v", n.PaidAt)
	}
}

// Review Focus 1: retry DOKU untuk invoice billing lama (tabel sudah dihapus) tidak boleh 500.
func TestDokuWebhookIgnoresRetiredBillingInvoice(t *testing.T) {
	p := &dokuProcessorStub{}
	router := newDokuWebhookTestRouter(p, dokuTestClientID, dokuTestSecretKey)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, signedDokuRequest(dokuBody("22222222-2222-2222-2222-222222222222", "SUCCESS"), dokuTestSecretKey))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"status":"ignored"`)) {
		t.Fatalf("body = %s, want status ignored", rec.Body.String())
	}
	if len(p.got) != 0 {
		t.Fatalf("processor must not be called, got %d", len(p.got))
	}
}

func TestDokuWebhookRejectsInvalidSignature(t *testing.T) {
	p := &dokuProcessorStub{}
	router := newDokuWebhookTestRouter(p, dokuTestClientID, dokuTestSecretKey)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, signedDokuRequest(dokuBody("RCV-1", "SUCCESS"), "wrong-secret"))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if len(p.got) != 0 {
		t.Fatal("processor must not be called on bad signature")
	}
}

func TestDokuWebhookRejectsMissingInvoiceNumber(t *testing.T) {
	router := newDokuWebhookTestRouter(&dokuProcessorStub{}, dokuTestClientID, dokuTestSecretKey)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, signedDokuRequest([]byte(`{"transaction":{"status":"SUCCESS"}}`), dokuTestSecretKey))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestDokuWebhookReturns503WhenNotConfigured(t *testing.T) {
	router := newDokuWebhookTestRouter(&dokuProcessorStub{}, "", "")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, signedDokuRequest(dokuBody("RCV-1", "SUCCESS"), dokuTestSecretKey))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestDokuWebhookProcessorErrorReturns500SoDokuRetries(t *testing.T) {
	p := &dokuProcessorStub{err: errors.New("db down")}
	router := newDokuWebhookTestRouter(p, dokuTestClientID, dokuTestSecretKey)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, signedDokuRequest(dokuBody("RCV-1", "SUCCESS"), dokuTestSecretKey))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
