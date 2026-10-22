package service

import (
	"context"
	"errors"
	"testing"

	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
)

var errInvoiceQuotaFull = errors.New("QUOTA_EXCEEDED")

// invoiceRoom simulates QuotaEnforcer: it fails while full is set and records
// every feature key it was asked about.
type invoiceRoom struct {
	full  bool
	calls []string
}

func (r *invoiceRoom) RequireRoom(_ context.Context, _ string, featureKey string) error {
	r.calls = append(r.calls, featureKey)
	if r.full {
		return errInvoiceQuotaFull
	}
	return nil
}

func harnessWithQuota(t *testing.T, room *invoiceRoom) *invoiceHarness {
	t.Helper()
	h := newInvoiceHarness(t)
	WithInvoiceQuotaEnforcer(room)(h.svc)
	return h
}

func manualInput() InvoiceInput {
	return InvoiceInput{AccountID: "a1", Lines: []LineInput{line("Instalasi", "500000")}}
}

func TestInvoiceCreateRejectedWhenMonthlyQuotaFull(t *testing.T) {
	room := &invoiceRoom{full: true}
	h := harnessWithQuota(t, room)

	_, err := h.svc.CreateDraft(ctx, scope, manualInput(), "u1")
	if !errors.Is(err, errInvoiceQuotaFull) {
		t.Fatalf("CreateDraft() error = %v, want QUOTA_EXCEEDED", err)
	}
	if len(h.store.invoices) != 0 {
		t.Error("invoice stored despite full quota")
	}
	if len(room.calls) != 1 || room.calls[0] != domain.FeatureReceivableMaxInvoicesPerMonth {
		t.Errorf("enforcer keys = %v", room.calls)
	}

	// Sales-order derived invoices (BillOrder / BillDelivery) are enforced too.
	so := manualInput()
	so.SourceType, so.SourceID, so.IdempotencyKey = domain.SourceSalesOrder, "so-1", "initial"
	if _, err := h.svc.CreateAndIssue(ctx, scope, so, "u1"); !errors.Is(err, errInvoiceQuotaFull) {
		t.Fatalf("CreateAndIssue(sales_order) error = %v, want QUOTA_EXCEEDED", err)
	}
	if len(h.store.invoices) != 0 {
		t.Error("sales order invoice stored despite full quota")
	}
}

func TestInvoiceCreateValidationRunsBeforeQuota(t *testing.T) {
	room := &invoiceRoom{full: true}
	h := harnessWithQuota(t, room)
	in := manualInput()
	in.Lines = nil
	if _, err := h.svc.CreateDraft(ctx, scope, in, "u1"); errors.Is(err, errInvoiceQuotaFull) || err == nil {
		t.Fatalf("CreateDraft() error = %v, want validation error", err)
	}
	if len(room.calls) != 0 {
		t.Error("enforcer consulted before validation")
	}
}

func TestInvoiceCreateAllowedWhenRoomAndWithoutEnforcer(t *testing.T) {
	room := &invoiceRoom{}
	h := harnessWithQuota(t, room)
	if _, err := h.svc.CreateDraft(ctx, scope, manualInput(), "u1"); err != nil {
		t.Fatalf("CreateDraft() with room error = %v", err)
	}
	legacy := newInvoiceHarness(t) // nil enforcer = no-op
	if _, err := legacy.svc.CreateDraft(ctx, scope, manualInput(), "u1"); err != nil {
		t.Fatalf("CreateDraft() without enforcer error = %v", err)
	}
}

// The recurring billing run is counted by the meter but never blocked:
// rejecting it would silently drop the tenant's billing for the period.
func TestRecurringBillingInvoiceNotBlockedWhenInvoiceQuotaFull(t *testing.T) {
	room := &invoiceRoom{full: true}
	h := harnessWithQuota(t, room)
	in := manualInput()
	in.SourceType, in.SourceID, in.ContractID, in.IdempotencyKey = domain.SourceContract, "ct-1", "ct-1", "period:2026-10-05"

	inv, err := h.svc.CreateAndIssue(ctx, scope, in, "")
	if err != nil {
		t.Fatalf("CreateAndIssue(contract) error = %v, want success despite full quota", err)
	}
	if inv.Status != domain.InvoiceIssued {
		t.Errorf("status = %s, want issued", inv.Status)
	}
	if len(room.calls) != 0 {
		t.Errorf("enforcer consulted for a billing-run invoice: %v", room.calls)
	}
}

func TestInvoicePaymentNotBlockedWhenInvoiceQuotaFull(t *testing.T) {
	room := &invoiceRoom{}
	h := harnessWithQuota(t, room)
	inv := issuedInvoice(t, h, "333000")

	room.full = true
	room.calls = nil
	if _, _, err := h.pay.RecordManual(ctx, scope, inv.ID, ManualPaymentInput{Amount: "100000", Reference: "TRF-1"}, "u1"); err != nil {
		t.Fatalf("RecordManual() error = %v, want success despite full quota", err)
	}
	paid, _, err := h.pay.RecordManual(ctx, scope, inv.ID, ManualPaymentInput{Amount: "233000"}, "u1")
	if err != nil || paid.Status != domain.InvoicePaid {
		t.Fatalf("final payment = %+v err=%v", paid, err)
	}
	if len(room.calls) != 0 {
		t.Errorf("enforcer consulted on payment: %v", room.calls)
	}
}

// The receivable module has no credit-note or refund feature (a DOKU payment on
// a non-payable invoice is only logged for manual refund). The closest
// post-creation operations that must keep working for an invoice created
// earlier are: provider (DOKU) payments, issuing an existing draft, and void.
func TestCreditNoteAndRefundNotBlockedWhenInvoiceQuotaFull(t *testing.T) {
	room := &invoiceRoom{}
	h := harnessWithQuota(t, room)
	draft := h.draft(t, InvoiceInput{Lines: []LineInput{line("Paket", "100000")}})
	toVoid := issuedInvoice(t, h, "200000")
	toPay := issuedInvoice(t, h, "300000")
	editable := h.draft(t, InvoiceInput{})

	room.full = true
	room.calls = nil
	if _, err := h.svc.Issue(ctx, scope, draft.ID, "u1"); err != nil {
		t.Fatalf("Issue() existing draft error = %v", err)
	}
	if _, err := h.svc.Void(ctx, scope, toVoid.ID, "salah input", "u1"); err != nil {
		t.Fatalf("Void() error = %v", err)
	}
	if _, err := h.svc.UpdateDraft(ctx, scope, editable.ID, manualInput(), "u1"); err != nil {
		t.Fatalf("UpdateDraft() error = %v", err)
	}
	if _, err := h.pay.RecordProvider(ctx, scope, toPay.ID, repository.PaymentParams{Amount: "300000", Method: "doku", Reference: "DOKU-1", PaidAt: nowWIB}); err != nil {
		t.Fatalf("RecordProvider() error = %v", err)
	}
	if len(room.calls) != 0 {
		t.Errorf("enforcer consulted outside creation: %v", room.calls)
	}
}
