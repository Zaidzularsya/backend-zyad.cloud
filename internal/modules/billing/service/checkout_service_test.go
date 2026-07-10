package service

import (
	"context"
	"errors"
	"testing"
	"time"

	billing "zyad.cloud/internal/modules/billing"
	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/platform/doku"
)

type stubDokuClient struct {
	requests []doku.CreatePaymentRequest
	payment  doku.Payment
	err      error
}

func (s *stubDokuClient) CreatePayment(
	_ context.Context,
	request doku.CreatePaymentRequest,
) (doku.Payment, error) {
	s.requests = append(s.requests, request)
	return s.payment, s.err
}

func newCheckoutServiceForTest(
	invoice model.Invoice,
	store *stubPaymentStore,
	client *stubDokuClient,
) *PaymentService {
	service := NewPaymentService(store, &stubPaymentInvoiceStore{invoice: invoice}, nil)
	service.SetDokuCheckout(client, "https://app.example.com/")
	service.now = func() time.Time { return time.Date(2026, 7, 9, 1, 0, 0, 0, time.UTC) }
	return service
}

func TestCreateCheckoutCreatesPendingPayment(t *testing.T) {
	store := &stubPaymentStore{}
	client := &stubDokuClient{
		payment: doku.Payment{
			TokenID:     "token-1",
			SessionID:   "session-1",
			PaymentURL:  "https://sandbox.doku.com/checkout/link/abc",
			ExpiredDate: time.Date(2026, 7, 9, 2, 0, 0, 0, time.UTC),
		},
	}
	invoice := model.Invoice{
		ID:             "invoice-1",
		OrganizationID: "organization-1",
		Status:         model.InvoiceStatusOpen,
		TotalAmount:    "199000.00",
		Currency:       "IDR",
	}
	service := newCheckoutServiceForTest(invoice, store, client)

	result, err := service.CreateCheckout(context.Background(), "organization-1", "invoice-1")
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	if result.PaymentURL != "https://sandbox.doku.com/checkout/link/abc" || result.Provider != "doku" {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 1 {
		t.Fatalf("doku requests = %d, want 1", len(client.requests))
	}
	request := client.requests[0]
	if request.InvoiceNumber != "invoice-1" || request.Amount != 199000 || request.Currency != "IDR" {
		t.Fatalf("doku request = %#v", request)
	}
	if request.CallbackURL != "https://app.example.com/app/checkout/success?invoice=invoice-1" {
		t.Fatalf("callback url = %q", request.CallbackURL)
	}
	if len(store.createdPayments) != 1 {
		t.Fatalf("created payments = %d, want 1", len(store.createdPayments))
	}
	created := store.createdPayments[0]
	if created.Provider != model.PaymentProviderDoku ||
		created.Status != model.PaymentStatusPending ||
		created.ProviderReference != "token-1" ||
		created.RawPayload["payment_url"] != "https://sandbox.doku.com/checkout/link/abc" {
		t.Fatalf("created payment = %#v", created)
	}
}

func TestCreateCheckoutReusesPendingUnexpiredSession(t *testing.T) {
	store := &stubPaymentStore{
		existingPayments: []model.Payment{
			{
				ID:             "payment-1",
				InvoiceID:      "invoice-1",
				OrganizationID: "organization-1",
				Provider:       model.PaymentProviderDoku,
				Status:         model.PaymentStatusPending,
				RawPayload: map[string]any{
					"payment_url":  "https://sandbox.doku.com/checkout/link/existing",
					"expired_date": "2026-07-09T02:00:00Z",
				},
			},
		},
	}
	client := &stubDokuClient{}
	invoice := model.Invoice{
		ID:             "invoice-1",
		OrganizationID: "organization-1",
		Status:         model.InvoiceStatusOpen,
		TotalAmount:    "199000.00",
	}
	service := newCheckoutServiceForTest(invoice, store, client)

	result, err := service.CreateCheckout(context.Background(), "organization-1", "invoice-1")
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	if result.PaymentURL != "https://sandbox.doku.com/checkout/link/existing" {
		t.Fatalf("result = %#v", result)
	}
	if len(client.requests) != 0 {
		t.Fatalf("doku requests = %d, want 0 (session should be reused)", len(client.requests))
	}
}

func TestCreateCheckoutRejectsPaidInvoice(t *testing.T) {
	paidAt := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	invoice := model.Invoice{
		ID:             "invoice-1",
		OrganizationID: "organization-1",
		Status:         model.InvoiceStatusPaid,
		PaidAt:         &paidAt,
		TotalAmount:    "199000.00",
	}
	service := newCheckoutServiceForTest(invoice, &stubPaymentStore{}, &stubDokuClient{})

	_, err := service.CreateCheckout(context.Background(), "organization-1", "invoice-1")
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != billing.ErrCodePaymentAlreadyProcessed {
		t.Fatalf("CreateCheckout() error = %v, want PAYMENT_ALREADY_PROCESSED", err)
	}
}

func TestCreateCheckoutWithoutClientIsUnavailable(t *testing.T) {
	service := NewPaymentService(&stubPaymentStore{}, &stubPaymentInvoiceStore{}, nil)

	_, err := service.CreateCheckout(context.Background(), "organization-1", "invoice-1")
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Status != 503 {
		t.Fatalf("CreateCheckout() error = %v, want 503", err)
	}
}

func TestWholeCurrencyAmountRounds(t *testing.T) {
	cases := map[string]int64{
		"199000.00": 199000,
		"199000.49": 199000,
		"199000.50": 199001,
		"0.00":      0,
	}
	for input, want := range cases {
		got, err := wholeCurrencyAmount(input)
		if err != nil || got != want {
			t.Fatalf("wholeCurrencyAmount(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	if _, err := wholeCurrencyAmount("-1"); err == nil {
		t.Fatal("wholeCurrencyAmount(-1) error = nil, want error")
	}
}
