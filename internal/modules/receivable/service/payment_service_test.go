package service

import (
	"context"
	"errors"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
)

func issuedInvoice(t *testing.T, h *invoiceHarness, price string) domain.Invoice {
	t.Helper()
	inv, err := h.svc.Issue(ctx, scope, h.draft(t, InvoiceInput{Lines: []LineInput{line("Paket", price)}}).ID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestManualPaymentCompletesAndNotifiesListener(t *testing.T) {
	h := newInvoiceHarness(t)
	inv := issuedInvoice(t, h, "333000")
	renders := len(h.renderer.docs)

	part, pay, err := h.pay.RecordManual(ctx, scope, inv.ID, ManualPaymentInput{Amount: "100000", Reference: "TRF-1"}, "u1")
	if err != nil || part.Status != domain.InvoiceIssued || pay.Amount != "100000.00" || len(h.listener.paid) != 0 {
		t.Fatalf("partial = %+v pay=%+v err=%v", part, pay, err)
	}
	if len(h.renderer.docs) != renders {
		t.Fatal("partial payment must not refresh the snapshot")
	}
	if _, _, err := h.pay.RecordManual(ctx, scope, inv.ID, ManualPaymentInput{Amount: "300000"}, "u1"); !errors.Is(err, repository.ErrPaymentExceeds) {
		t.Fatalf("overpay err = %v", err)
	}
	paid, _, err := h.pay.RecordManual(ctx, scope, inv.ID, ManualPaymentInput{Amount: "233000"}, "u1")
	if err != nil || paid.Status != domain.InvoicePaid {
		t.Fatalf("paid = %+v err=%v", paid, err)
	}
	if len(h.listener.paid) != 1 || h.listener.paid[0].ID != inv.ID || h.listener.paid[0].Number != "INV-2026-0001" {
		t.Fatalf("listener = %+v", h.listener.paid)
	}
	if last := h.renderer.docs[len(h.renderer.docs)-1]; last.Stamp != "LUNAS" || len(h.renderer.docs) != renders+1 {
		t.Fatalf("snapshot must be regenerated once with LUNAS, stamp=%q renders %d→%d", last.Stamp, renders, len(h.renderer.docs))
	}
	if h.files.deletes != 1 {
		t.Fatalf("old snapshot must be deleted, deletes = %d", h.files.deletes)
	}
}

func TestManualPaymentValidation(t *testing.T) {
	h := newInvoiceHarness(t)
	inv := issuedInvoice(t, h, "333000")
	for _, in := range []ManualPaymentInput{
		{Amount: "0"}, {Amount: "-1"}, {Amount: "abc"}, {Amount: ""}, {Amount: "1.234"},
		{Amount: "1000", PaidAt: nowWIB.Add(24 * time.Hour)},
	} {
		if _, _, err := h.pay.RecordManual(ctx, scope, inv.ID, in, "u1"); !errors.Is(err, ErrInvalidPayment) {
			t.Errorf("%+v: err = %v, want ErrInvalidPayment", in, err)
		}
	}
	if len(h.store.payments) != 0 {
		t.Fatal("invalid payments must not reach the repository")
	}
	got, pay, err := h.pay.RecordManual(ctx, scope, inv.ID, ManualPaymentInput{Amount: "1000"}, "u1")
	if err != nil || !pay.PaidAt.Equal(nowWIB) || got.AmountPaid != "1000.00" {
		t.Fatalf("default paid_at: pay=%+v err=%v", pay, err)
	}
}

func TestProviderPaymentIdempotentByReference(t *testing.T) {
	h := newInvoiceHarness(t)
	inv := issuedInvoice(t, h, "100000")
	p := repository.PaymentParams{Amount: "100000", Method: "doku", Reference: "R1", PaidAt: nowWIB}
	first, err := h.pay.RecordProvider(ctx, scope, inv.ID, p)
	if err != nil || first.Status != domain.InvoicePaid {
		t.Fatalf("first = %+v err=%v", first, err)
	}
	if _, err := h.pay.RecordProvider(ctx, scope, inv.ID, p); err != nil {
		t.Fatalf("retry must be a no-op, err = %v", err)
	}
	if len(h.listener.paid) != 1 || len(h.store.payments) != 1 {
		t.Fatalf("listener=%d payments=%d, want 1/1", len(h.listener.paid), len(h.store.payments))
	}
	if _, err := h.pay.RecordProvider(ctx, scope, inv.ID, repository.PaymentParams{Amount: "1", Method: "manual", PaidAt: nowWIB}); !errors.Is(err, ErrInvalidPayment) {
		t.Fatalf("provider path must reject method manual, err = %v", err)
	}
	if _, err := h.pay.RecordProvider(ctx, scope, inv.ID, repository.PaymentParams{Amount: "1", Method: "doku", PaidAt: nowWIB}); !errors.Is(err, ErrInvalidPayment) {
		t.Fatalf("provider path requires a reference, err = %v", err)
	}
}

func TestListenerPanicDoesNotFailPayment(t *testing.T) {
	h := newInvoiceHarness(t)
	reg := NewRegistry()
	reg.Add(panicListener{})
	reg.Add(h.listener)
	pay := NewPaymentService(PaymentDeps{Payments: fakePayments{h.store}, Invoices: h.svc, Listeners: reg, Now: func() time.Time { return nowWIB }})
	inv := issuedInvoice(t, h, "1000")
	if got, _, err := pay.RecordManual(ctx, scope, inv.ID, ManualPaymentInput{Amount: "1000"}, "u1"); err != nil || got.Status != domain.InvoicePaid {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if len(h.listener.paid) != 1 {
		t.Fatal("listeners after a panicking one must still run")
	}
}

type panicListener struct{}

func (panicListener) InvoicePaid(_ context.Context, _ coretenant.Scope, _ InvoiceRef) { panic("boom") }
func (panicListener) ContractCreated(_ context.Context, _ coretenant.Scope, _ ContractRef) {
	panic("boom")
}
