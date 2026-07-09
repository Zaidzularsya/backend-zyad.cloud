package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/modules/billing/repository"
)

type stubPaymentStore struct {
	existingPayments []model.Payment
	createdPayments  []repository.CreatePaymentParams
	createdEvents    []repository.PaymentEventParams
	existingEvent    model.PaymentEvent
	createEventErr   error
}

func (s *stubPaymentStore) Create(
	_ context.Context,
	params repository.CreatePaymentParams,
) (model.Payment, error) {
	s.createdPayments = append(s.createdPayments, params)
	return model.Payment{
		ID:                "payment-1",
		InvoiceID:         params.InvoiceID,
		OrganizationID:    params.OrganizationID,
		Provider:          params.Provider,
		ProviderReference: params.ProviderReference,
		PaymentMethod:     params.PaymentMethod,
		Status:            params.Status,
		Amount:            params.Amount,
		Currency:          params.Currency,
		PaidAt:            params.PaidAt,
		RawPayload:        params.RawPayload,
	}, nil
}

func (s *stubPaymentStore) FindByID(context.Context, string, string) (model.Payment, error) {
	return model.Payment{}, nil
}

func (s *stubPaymentStore) FindEventByProviderEventID(
	context.Context,
	model.PaymentProvider,
	string,
) (model.PaymentEvent, error) {
	if s.existingEvent.ID == "" {
		return model.PaymentEvent{}, errors.New("payment event not found")
	}
	return s.existingEvent, nil
}

func (s *stubPaymentStore) List(
	context.Context,
	repository.PaymentListFilter,
) ([]model.Payment, int64, error) {
	return s.existingPayments, int64(len(s.existingPayments)), nil
}

func (s *stubPaymentStore) CreateEvent(
	_ context.Context,
	params repository.PaymentEventParams,
) (model.PaymentEvent, error) {
	if s.createEventErr != nil {
		return model.PaymentEvent{}, s.createEventErr
	}
	s.createdEvents = append(s.createdEvents, params)
	return model.PaymentEvent{
		ID:              "payment-event-1",
		PaymentID:       params.PaymentID,
		InvoiceID:       params.InvoiceID,
		Provider:        params.Provider,
		Type:            params.Type,
		ProviderEventID: params.ProviderEventID,
		Payload:         params.Payload,
		ProcessedAt:     params.ProcessedAt,
		CreatedAt:       time.Date(2026, 6, 30, 8, 0, 0, 0, time.UTC),
	}, nil
}

type stubPaymentInvoiceStore struct {
	invoice     model.Invoice
	updateCalls int
	lastUpdate  repository.UpdateInvoiceStatusParams
}

func (s *stubPaymentInvoiceStore) FindByID(context.Context, string, string) (model.Invoice, error) {
	return s.invoice, nil
}

type stubPaymentSubscriptionUpgradeActivator struct {
	calls       int
	lastInvoice SubscriptionUpgradeInvoice
	lastPaidAt  time.Time
}

func (s *stubPaymentSubscriptionUpgradeActivator) ActivateUpgradeByInvoice(
	_ context.Context,
	invoice SubscriptionUpgradeInvoice,
	paidAt time.Time,
) error {
	s.calls++
	s.lastInvoice = invoice
	s.lastPaidAt = paidAt
	return nil
}

func (s *stubPaymentInvoiceStore) FindByIDUnscoped(context.Context, string) (model.Invoice, error) {
	return s.invoice, nil
}

func (s *stubPaymentInvoiceStore) UpdateStatus(
	_ context.Context,
	params repository.UpdateInvoiceStatusParams,
) (model.Invoice, error) {
	s.updateCalls++
	s.lastUpdate = params
	s.invoice.Status = params.Status
	s.invoice.PaidAt = params.PaidAt
	return s.invoice, nil
}

func TestPaymentServiceMarkInvoicePaidCreatesPaymentEventAndUpdatesInvoice(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	paymentStore := &stubPaymentStore{}
	invoiceStore := &stubPaymentInvoiceStore{
		invoice: model.Invoice{
			ID:             "invoice-1",
			OrganizationID: "organization-1",
			Status:         model.InvoiceStatusOpen,
			TotalAmount:    "111000.00",
			Currency:       "IDR",
		},
	}
	service := NewPaymentService(paymentStore, invoiceStore, nil)
	service.now = func() time.Time { return now }

	response, err := service.MarkInvoicePaid(context.Background(), "organization-1", "invoice-1", dto.MarkInvoicePaidRequest{
		PaymentMethod: "bank_transfer",
	})
	if err != nil {
		t.Fatalf("MarkInvoicePaid error = %v", err)
	}
	if response.Status != "paid" || response.Amount != "111000.00" {
		t.Fatalf("response = %#v", response)
	}
	if len(paymentStore.createdPayments) != 1 {
		t.Fatalf("created payments = %d, want 1", len(paymentStore.createdPayments))
	}
	if paymentStore.createdPayments[0].ProviderReference != "manual-invoice-1" {
		t.Fatalf("provider reference = %s, want manual-invoice-1", paymentStore.createdPayments[0].ProviderReference)
	}
	if len(paymentStore.createdEvents) != 1 {
		t.Fatalf("created events = %d, want 1", len(paymentStore.createdEvents))
	}
	if paymentStore.createdEvents[0].ProviderEventID != "manual-invoice-1:paid" {
		t.Fatalf("provider event id = %s, want manual-invoice-1:paid", paymentStore.createdEvents[0].ProviderEventID)
	}
	if invoiceStore.updateCalls != 1 || invoiceStore.lastUpdate.Status != model.InvoiceStatusPaid {
		t.Fatalf("invoice update = %#v calls=%d", invoiceStore.lastUpdate, invoiceStore.updateCalls)
	}
}

func TestPaymentServiceMarkInvoicePaidReturnsExistingPaidPayment(t *testing.T) {
	paidAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	paymentStore := &stubPaymentStore{
		existingPayments: []model.Payment{
			{
				ID:             "payment-existing",
				InvoiceID:      "invoice-1",
				OrganizationID: "organization-1",
				Provider:       model.PaymentProviderManual,
				Status:         model.PaymentStatusPaid,
				Amount:         "111000.00",
				Currency:       "IDR",
				PaidAt:         &paidAt,
			},
		},
	}
	invoiceStore := &stubPaymentInvoiceStore{
		invoice: model.Invoice{
			ID:             "invoice-1",
			OrganizationID: "organization-1",
			Status:         model.InvoiceStatusPaid,
			TotalAmount:    "111000.00",
			Currency:       "IDR",
			PaidAt:         &paidAt,
		},
	}
	service := NewPaymentService(paymentStore, invoiceStore, nil)

	response, err := service.MarkInvoicePaid(context.Background(), "organization-1", "invoice-1", dto.MarkInvoicePaidRequest{})
	if err != nil {
		t.Fatalf("MarkInvoicePaid error = %v", err)
	}
	if response.ID != "payment-existing" {
		t.Fatalf("response ID = %s, want payment-existing", response.ID)
	}
	if len(paymentStore.createdPayments) != 0 {
		t.Fatalf("created payments = %d, want 0", len(paymentStore.createdPayments))
	}
	if len(paymentStore.createdEvents) != 0 {
		t.Fatalf("created events = %d, want 0", len(paymentStore.createdEvents))
	}
	if invoiceStore.updateCalls != 0 {
		t.Fatalf("invoice update calls = %d, want 0", invoiceStore.updateCalls)
	}
}

func TestPaymentServiceMarkInvoicePaidByIDUsesInvoiceOrganization(t *testing.T) {
	paymentStore := &stubPaymentStore{}
	invoiceStore := &stubPaymentInvoiceStore{
		invoice: model.Invoice{
			ID:             "invoice-1",
			OrganizationID: "organization-1",
			Status:         model.InvoiceStatusOpen,
			TotalAmount:    "111000.00",
			Currency:       "IDR",
		},
	}
	service := NewPaymentService(paymentStore, invoiceStore, nil)

	response, err := service.MarkInvoicePaidByID(context.Background(), "invoice-1", dto.MarkInvoicePaidRequest{})
	if err != nil {
		t.Fatalf("MarkInvoicePaidByID error = %v", err)
	}
	if response.OrganizationID != "organization-1" || response.InvoiceID != "invoice-1" {
		t.Fatalf("response = %#v", response)
	}
	if len(paymentStore.createdPayments) != 1 {
		t.Fatalf("created payments = %d, want 1", len(paymentStore.createdPayments))
	}
}

func TestPaymentServiceMarkInvoicePaidActivatesUpgradeInvoice(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	paymentStore := &stubPaymentStore{}
	invoiceStore := &stubPaymentInvoiceStore{
		invoice: model.Invoice{
			ID:             "invoice-upgrade-1",
			OrganizationID: "organization-1",
			SubscriptionID: stringPointer("subscription-1"),
			Status:         model.InvoiceStatusOpen,
			TotalAmount:    "249000.00",
			Currency:       "IDR",
			Metadata: map[string]any{
				"billing_action":   "upgrade_request",
				"target_plan_id":   "plan-pro",
				"billing_interval": "monthly",
			},
		},
	}
	activator := &stubPaymentSubscriptionUpgradeActivator{}
	service := NewPaymentService(paymentStore, invoiceStore, activator)
	service.now = func() time.Time { return now }

	response, err := service.MarkInvoicePaid(context.Background(), "organization-1", "invoice-upgrade-1", dto.MarkInvoicePaidRequest{})
	if err != nil {
		t.Fatalf("MarkInvoicePaid error = %v", err)
	}
	if response.Status != "paid" {
		t.Fatalf("response status = %s, want paid", response.Status)
	}
	if activator.calls != 1 {
		t.Fatalf("upgrade activator calls = %d, want 1", activator.calls)
	}
	if activator.lastInvoice.OrganizationID != "organization-1" ||
		activator.lastInvoice.SubscriptionID == nil ||
		*activator.lastInvoice.SubscriptionID != "subscription-1" {
		t.Fatalf("last invoice = %#v", activator.lastInvoice)
	}
	if !activator.lastPaidAt.Equal(now) {
		t.Fatalf("last paid at = %s, want %s", activator.lastPaidAt, now)
	}
}

func TestPaymentServiceRecordProviderEventCreatesNewEvent(t *testing.T) {
	processedAt := "2026-06-30T10:00:00Z"
	paymentStore := &stubPaymentStore{}
	service := NewPaymentService(paymentStore, nil, nil)

	response, err := service.RecordProviderEvent(context.Background(), dto.RecordPaymentProviderEventRequest{
		PaymentID:       "payment-1",
		InvoiceID:       "invoice-1",
		Provider:        "xendit",
		EventType:       "invoice.paid",
		ProviderEventID: "evt-123",
		ProcessedAt:     &processedAt,
		Payload:         map[string]any{"status": "PAID"},
	})
	if err != nil {
		t.Fatalf("RecordProviderEvent error = %v", err)
	}
	if response.Provider != "xendit" || response.EventType != "invoice.paid" || response.ProviderEventID != "evt-123" {
		t.Fatalf("response = %#v", response)
	}
	if len(paymentStore.createdEvents) != 1 {
		t.Fatalf("created events = %d, want 1", len(paymentStore.createdEvents))
	}
}

func TestPaymentServiceRecordProviderEventReturnsExistingOnDuplicate(t *testing.T) {
	processedAt := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	paymentStore := &stubPaymentStore{
		createEventErr: &pgconn.PgError{Code: "23505"},
		existingEvent: model.PaymentEvent{
			ID:              "payment-event-existing",
			PaymentID:       stringPointer("payment-1"),
			InvoiceID:       stringPointer("invoice-1"),
			Provider:        model.PaymentProviderXendit,
			Type:            "invoice.paid",
			ProviderEventID: "evt-123",
			Payload:         map[string]any{"status": "PAID"},
			ProcessedAt:     &processedAt,
			CreatedAt:       time.Date(2026, 6, 30, 8, 0, 0, 0, time.UTC),
		},
	}
	service := NewPaymentService(paymentStore, nil, nil)

	response, err := service.RecordProviderEvent(context.Background(), dto.RecordPaymentProviderEventRequest{
		Provider:        "xendit",
		EventType:       "invoice.paid",
		ProviderEventID: "evt-123",
		Payload:         map[string]any{"status": "PAID"},
	})
	if err != nil {
		t.Fatalf("RecordProviderEvent error = %v", err)
	}
	if response.ID != "payment-event-existing" || response.ProviderEventID != "evt-123" {
		t.Fatalf("response = %#v", response)
	}
	if len(paymentStore.createdEvents) != 0 {
		t.Fatalf("created events = %d, want 0 because duplicate path returned existing", len(paymentStore.createdEvents))
	}
}
