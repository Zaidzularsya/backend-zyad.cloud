package service

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
)

type voidedRecorder struct {
	recordingListener
	voided []InvoiceRef
	panics bool
}

func (v *voidedRecorder) InvoiceVoided(_ context.Context, _ coretenant.Scope, inv InvoiceRef) {
	v.voided = append(v.voided, inv)
	if v.panics {
		panic("boom")
	}
}

func TestRegistry_InvoiceVoided_OnlyVoidedListeners_PanicSwallowed(t *testing.T) {
	r := NewRegistry()
	plain := &recordingListener{}
	bad := &voidedRecorder{panics: true}
	good := &voidedRecorder{}
	r.Add(plain)
	r.Add(bad)
	r.Add(good)

	r.InvoiceVoided(context.Background(), scope, InvoiceRef{ID: "inv-1"})

	if len(bad.voided) != 1 || len(good.voided) != 1 || good.voided[0].ID != "inv-1" {
		t.Fatalf("bad=%+v good=%+v", bad.voided, good.voided)
	}
	if len(plain.paid) != 0 {
		t.Fatalf("plain listener touched: %+v", plain)
	}
	var nilReg *Registry
	nilReg.InvoiceVoided(context.Background(), scope, InvoiceRef{ID: "x"}) // nil-safe
}

func TestInvoiceService_Void_NotifiesVoidedListeners(t *testing.T) {
	h := newInvoiceHarness(t)
	vl := &voidedRecorder{}
	// Registry is shared by reference with the service.
	h.svc.listeners.Add(vl)
	inv := h.draft(t, InvoiceInput{})
	if _, err := h.svc.Void(ctx, scope, inv.ID, "salah input", "u1"); err != nil {
		t.Fatalf("Void: %v", err)
	}
	if len(vl.voided) != 1 || vl.voided[0].ID != inv.ID {
		t.Fatalf("voided = %+v", vl.voided)
	}
}
