package doku

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSignatureIsDeterministicAndVerifiable(t *testing.T) {
	body := []byte(`{"order":{"amount":199000,"invoice_number":"inv-1"}}`)
	digest := Digest(body)
	signature := Signature("BRN-0001", "req-1", "2026-07-09T01:02:03Z", "/checkout/v1/payment", digest, "SK-secret")

	if !strings.HasPrefix(signature, "HMACSHA256=") {
		t.Fatalf("signature = %q, want HMACSHA256= prefix", signature)
	}
	if !VerifySignature("BRN-0001", "req-1", "2026-07-09T01:02:03Z", "/checkout/v1/payment", body, signature, "SK-secret") {
		t.Fatal("VerifySignature() = false for a signature produced by Signature()")
	}
	if VerifySignature("BRN-0001", "req-1", "2026-07-09T01:02:03Z", "/checkout/v1/payment", body, signature, "SK-wrong") {
		t.Fatal("VerifySignature() = true with wrong secret")
	}
	if VerifySignature("BRN-0001", "req-1", "2026-07-09T01:02:03Z", "/checkout/v1/payment", []byte("tampered"), signature, "SK-secret") {
		t.Fatal("VerifySignature() = true with tampered body")
	}
}

func TestHTTPClientCreatePaymentSignsAndParsesResponse(t *testing.T) {
	var received struct {
		clientID  string
		requestID string
		timestamp string
		digest    string
		signature string
		body      []byte
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.clientID = r.Header.Get("Client-Id")
		received.requestID = r.Header.Get("Request-Id")
		received.timestamp = r.Header.Get("Request-Timestamp")
		received.digest = r.Header.Get("Digest")
		received.signature = r.Header.Get("Signature")
		received.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"message": ["SUCCESS"],
			"response": {
				"order": {"invoice_number": "inv-1", "session_id": "session-1"},
				"payment": {
					"token_id": "token-1",
					"url": "https://sandbox.doku.com/checkout/link/abc",
					"expired_date": "20260709120000",
					"payment_due_date": 60
				}
			}
		}`))
	}))
	defer server.Close()

	client := NewHTTPClient(Config{
		BaseURL:   server.URL,
		ClientID:  "BRN-0001",
		SecretKey: "SK-secret",
	})
	client.now = func() time.Time { return time.Date(2026, 7, 9, 1, 2, 3, 0, time.UTC) }
	client.newUUID = func() string { return "req-fixed" }

	payment, err := client.CreatePayment(context.Background(), CreatePaymentRequest{
		InvoiceNumber: "inv-1",
		Amount:        199000,
		Currency:      "IDR",
		CallbackURL:   "https://app.example.com/app/checkout/success?invoice=inv-1",
	})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}

	if received.clientID != "BRN-0001" || received.requestID != "req-fixed" ||
		received.timestamp != "2026-07-09T01:02:03Z" {
		t.Fatalf("headers = %+v", received)
	}
	if received.digest != Digest(received.body) {
		t.Fatalf("Digest header = %q, want %q", received.digest, Digest(received.body))
	}
	expectedSignature := Signature(
		"BRN-0001", "req-fixed", "2026-07-09T01:02:03Z",
		"/checkout/v1/payment", received.digest, "SK-secret",
	)
	if received.signature != expectedSignature {
		t.Fatalf("Signature header = %q, want %q", received.signature, expectedSignature)
	}

	var body map[string]any
	if err := json.Unmarshal(received.body, &body); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	order := body["order"].(map[string]any)
	if order["invoice_number"] != "inv-1" || order["amount"] != float64(199000) {
		t.Fatalf("order = %#v", order)
	}

	if payment.TokenID != "token-1" || payment.SessionID != "session-1" ||
		payment.PaymentURL != "https://sandbox.doku.com/checkout/link/abc" {
		t.Fatalf("payment = %#v", payment)
	}
	// expired_date is UTC+7: 2026-07-09 12:00:00 WIB == 05:00:00 UTC.
	if !payment.ExpiredDate.Equal(time.Date(2026, 7, 9, 5, 0, 0, 0, time.UTC)) {
		t.Fatalf("payment.ExpiredDate = %v", payment.ExpiredDate)
	}
}

func TestHTTPClientCreatePaymentSurfacesErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error_messages": ["order.amount is required"]}`))
	}))
	defer server.Close()

	client := NewHTTPClient(Config{BaseURL: server.URL, ClientID: "BRN-0001", SecretKey: "SK-secret"})
	_, err := client.CreatePayment(context.Background(), CreatePaymentRequest{
		InvoiceNumber: "inv-1",
		Amount:        1000,
	})
	if err == nil || !strings.Contains(err.Error(), "order.amount is required") {
		t.Fatalf("CreatePayment() error = %v, want doku error message", err)
	}
}

func TestNoopClientFabricatesSuccessRedirect(t *testing.T) {
	client := NewNoopClient("https://app.example.com/")
	payment, err := client.CreatePayment(context.Background(), CreatePaymentRequest{InvoiceNumber: "inv-1"})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}
	want := "https://app.example.com/app/checkout/success?invoice=inv-1&simulated=true"
	if payment.PaymentURL != want {
		t.Fatalf("payment url = %q, want %q", payment.PaymentURL, want)
	}
	if payment.ExpiredDate.IsZero() {
		t.Fatal("payment.ExpiredDate is zero")
	}
}

func TestNewClientFromConfigSelectsImplementation(t *testing.T) {
	if _, ok := NewClientFromConfig(Config{ClientID: "BRN-1", SecretKey: "SK-1"}).(*HTTPClient); !ok {
		t.Fatal("expected HTTPClient when credentials configured")
	}
	if _, ok := NewClientFromConfig(Config{FrontendURL: "http://localhost"}).(NoopClient); !ok {
		t.Fatal("expected NoopClient without credentials")
	}
}
