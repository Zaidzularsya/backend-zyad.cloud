package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	billing "zyad.cloud/internal/modules/billing"
	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/platform/doku"
)

const (
	dokuTestClientID  = "BRN-0001-TEST"
	dokuTestSecretKey = "SK-test-secret"
	dokuWebhookPath   = "/api/v1/webhooks/doku"
)

type dokuWebhookPaymentServiceStub struct {
	recordedEvents []dto.RecordPaymentProviderEventRequest
	recordErr      error
	paidInvoiceIDs []string
	paidRequests   []dto.MarkInvoicePaidRequest
	markPaidErr    error
}

func (s *dokuWebhookPaymentServiceStub) RecordProviderEvent(
	_ context.Context,
	request dto.RecordPaymentProviderEventRequest,
) (dto.PaymentEventResponse, error) {
	s.recordedEvents = append(s.recordedEvents, request)
	return dto.PaymentEventResponse{ID: "event-1"}, s.recordErr
}

func (s *dokuWebhookPaymentServiceStub) MarkInvoicePaidByID(
	_ context.Context,
	invoiceID string,
	request dto.MarkInvoicePaidRequest,
) (dto.PaymentResponse, error) {
	s.paidInvoiceIDs = append(s.paidInvoiceIDs, invoiceID)
	s.paidRequests = append(s.paidRequests, request)
	return dto.PaymentResponse{ID: "payment-1"}, s.markPaidErr
}

func newDokuWebhookTestRouter(payments DokuWebhookPaymentService, clientID, secretKey string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	NewDokuWebhookHandler(payments, clientID, secretKey, nil).RegisterRoutes(group)
	return router
}

func signedDokuRequest(t *testing.T, body []byte, secretKey string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, dokuWebhookPath, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Client-Id", dokuTestClientID)
	request.Header.Set("Request-Id", "notify-1")
	request.Header.Set("Request-Timestamp", "2026-07-09T01:02:03Z")
	request.Header.Set("Digest", doku.Digest(body))
	request.Header.Set("Signature", doku.Signature(
		dokuTestClientID,
		"notify-1",
		"2026-07-09T01:02:03Z",
		dokuWebhookPath,
		doku.Digest(body),
		secretKey,
	))
	return request
}

const dokuSuccessNotification = `{
	"service": {"id": "VIRTUAL_ACCOUNT"},
	"channel": {"id": "VIRTUAL_ACCOUNT_BCA"},
	"order": {"invoice_number": "22222222-2222-2222-2222-222222222222", "amount": 199000},
	"transaction": {
		"status": "SUCCESS",
		"date": "2026-07-09T01:02:00Z",
		"original_request_id": "req-original-1"
	}
}`

func TestDokuWebhookProcessesSuccessNotification(t *testing.T) {
	payments := &dokuWebhookPaymentServiceStub{}
	router := newDokuWebhookTestRouter(payments, dokuTestClientID, dokuTestSecretKey)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedDokuRequest(t, []byte(dokuSuccessNotification), dokuTestSecretKey))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if len(payments.recordedEvents) != 1 ||
		payments.recordedEvents[0].ProviderEventID != "webhook:22222222-2222-2222-2222-222222222222:success" ||
		payments.recordedEvents[0].Provider != "doku" {
		t.Fatalf("recorded events = %#v", payments.recordedEvents)
	}
	if len(payments.paidInvoiceIDs) != 1 ||
		payments.paidInvoiceIDs[0] != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("paid invoices = %#v", payments.paidInvoiceIDs)
	}
	paid := payments.paidRequests[0]
	if paid.Provider != "doku" || paid.Amount != "199000" ||
		paid.PaymentMethod != "VIRTUAL_ACCOUNT_BCA" || paid.ProviderReference != "req-original-1" {
		t.Fatalf("mark paid request = %#v", paid)
	}
	if paid.PaidAt == nil || *paid.PaidAt != "2026-07-09T01:02:00Z" {
		t.Fatalf("paid at = %v", paid.PaidAt)
	}
}

func TestDokuWebhookRejectsInvalidSignature(t *testing.T) {
	payments := &dokuWebhookPaymentServiceStub{}
	router := newDokuWebhookTestRouter(payments, dokuTestClientID, dokuTestSecretKey)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedDokuRequest(t, []byte(dokuSuccessNotification), "SK-wrong-secret"))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
	if len(payments.recordedEvents) != 0 || len(payments.paidInvoiceIDs) != 0 {
		t.Fatal("payload must not be processed when the signature is invalid")
	}
}

func TestDokuWebhookWithoutCredentialsIsUnavailable(t *testing.T) {
	router := newDokuWebhookTestRouter(&dokuWebhookPaymentServiceStub{}, "", "")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedDokuRequest(t, []byte(dokuSuccessNotification), dokuTestSecretKey))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestDokuWebhookRequiresInvoiceNumber(t *testing.T) {
	payments := &dokuWebhookPaymentServiceStub{}
	router := newDokuWebhookTestRouter(payments, dokuTestClientID, dokuTestSecretKey)

	body := []byte(`{"transaction": {"status": "SUCCESS"}}`)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedDokuRequest(t, body, dokuTestSecretKey))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestDokuWebhookIsIdempotentOnRetries(t *testing.T) {
	payments := &dokuWebhookPaymentServiceStub{
		recordErr:   billing.PaymentEventAlreadyProcessedError(),
		markPaidErr: billing.PaymentAlreadyProcessedError(),
	}
	router := newDokuWebhookTestRouter(payments, dokuTestClientID, dokuTestSecretKey)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedDokuRequest(t, []byte(dokuSuccessNotification), dokuTestSecretKey))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for retried notification, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDokuWebhookRecordsNonSuccessWithoutSettling(t *testing.T) {
	payments := &dokuWebhookPaymentServiceStub{}
	router := newDokuWebhookTestRouter(payments, dokuTestClientID, dokuTestSecretKey)

	body := []byte(`{
		"order": {"invoice_number": "22222222-2222-2222-2222-222222222222", "amount": 199000},
		"transaction": {"status": "EXPIRED"}
	}`)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedDokuRequest(t, body, dokuTestSecretKey))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if len(payments.recordedEvents) != 1 || len(payments.paidInvoiceIDs) != 0 {
		t.Fatalf("events = %d, paid = %d; want event recorded without settling",
			len(payments.recordedEvents), len(payments.paidInvoiceIDs))
	}
}

type receivableProcessorStub struct {
	events    []DokuNotificationEvent
	processed bool
	err       error
}

func (s *receivableProcessorStub) HandleNotification(_ context.Context, n DokuNotificationEvent) (bool, error) {
	s.events = append(s.events, n)
	return s.processed, s.err
}

const dokuReceivableNotification = `{
	"channel": {"id": "VIRTUAL_ACCOUNT_BCA"},
	"order": {"invoice_number": "RCV-33333333-3333-3333-3333-333333333333", "amount": 833000},
	"transaction": {"status": "SUCCESS", "original_request_id": "req-rcv-1", "date": "2026-07-09T01:02:00Z"}
}`

func newReceivableDispatchRouter(payments DokuWebhookPaymentService, processor ReceivableDokuProcessor) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewDokuWebhookHandler(payments, dokuTestClientID, dokuTestSecretKey, nil)
	if processor != nil {
		h.SetReceivableProcessor(processor)
	}
	h.RegisterRoutes(router.Group("/api/v1"))
	return router
}

func TestDokuWebhookDispatchesReceivableNumbers(t *testing.T) {
	payments := &dokuWebhookPaymentServiceStub{}
	processor := &receivableProcessorStub{processed: true}
	router := newReceivableDispatchRouter(payments, processor)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedDokuRequest(t, []byte(dokuReceivableNotification), dokuTestSecretKey))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if len(payments.recordedEvents) != 0 || len(payments.paidInvoiceIDs) != 0 {
		t.Fatalf("billing must not handle RCV- numbers: events=%d paid=%d", len(payments.recordedEvents), len(payments.paidInvoiceIDs))
	}
	if len(processor.events) != 1 {
		t.Fatalf("processor events = %#v", processor.events)
	}
	got := processor.events[0]
	if got.InvoiceNumber != "RCV-33333333-3333-3333-3333-333333333333" || got.Status != "SUCCESS" || got.Amount != "833000" ||
		got.ProviderReference != "req-rcv-1" || got.PaymentMethod != "VIRTUAL_ACCOUNT_BCA" || got.PaidAt == nil || got.Payload == nil {
		t.Fatalf("event = %#v", got)
	}
}

func TestDokuWebhookReceivableUnprocessedAndErrors(t *testing.T) {
	payments := &dokuWebhookPaymentServiceStub{}
	router := newReceivableDispatchRouter(payments, &receivableProcessorStub{processed: false})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedDokuRequest(t, []byte(dokuReceivableNotification), dokuTestSecretKey))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unprocessed must still be 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	router = newReceivableDispatchRouter(payments, &receivableProcessorStub{err: errors.New("db down")})
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, signedDokuRequest(t, []byte(dokuReceivableNotification), dokuTestSecretKey))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("processor error must be 500 so DOKU retries, got %d", recorder.Code)
	}
}

func TestDokuWebhookBillingNumbersStillGoToBilling(t *testing.T) {
	payments := &dokuWebhookPaymentServiceStub{}
	processor := &receivableProcessorStub{processed: true}
	router := newReceivableDispatchRouter(payments, processor)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedDokuRequest(t, []byte(dokuSuccessNotification), dokuTestSecretKey))

	if recorder.Code != http.StatusOK || len(processor.events) != 0 || len(payments.paidInvoiceIDs) != 1 {
		t.Fatalf("status=%d processor=%d paid=%d", recorder.Code, len(processor.events), len(payments.paidInvoiceIDs))
	}
}
