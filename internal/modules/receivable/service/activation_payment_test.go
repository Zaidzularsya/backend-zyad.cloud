package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
)

// keyGate grants exactly the listed feature keys.
type keyGate map[string]bool

func (g keyGate) RequireFeature(_ context.Context, _ string, key string) error {
	if !g[key] {
		return errors.New("not entitled")
	}
	return nil
}

type activationCheckerFake struct {
	invoices map[string]bool
	err      error
	calls    []string
}

func (f *activationCheckerFake) IsActivationInvoice(_ context.Context, scope coretenant.Scope, invoiceID string) (bool, error) {
	f.calls = append(f.calls, scope.OrganizationID()+"|"+invoiceID)
	if f.err != nil {
		return false, f.err
	}
	return f.invoices[invoiceID], nil
}

func resellerGate() keyGate { return keyGate{"landing.page_activation": true} }

func TestCheckoutActivationInvoicePayableWithoutReceivableOnlinePayment(t *testing.T) {
	h := newOnlineHarness(t)
	checker := &activationCheckerFake{invoices: map[string]bool{"inv-1": true}}
	h.svc.gate = resellerGate()
	h.svc.WithActivationInvoices(checker)

	url, _, err := h.svc.Checkout(ctx, "tok")
	if err != nil || url == "" || h.doku.calls != 1 {
		t.Fatalf("activation invoice must be payable online: url=%q calls=%d err=%v", url, h.doku.calls, err)
	}
	if len(checker.calls) != 1 || checker.calls[0] != "11111111-1111-1111-1111-111111111111|inv-1" {
		t.Fatalf("checker must be asked with the link's org scope, got %v", checker.calls)
	}
}

func TestCheckoutNonActivationInvoiceStillNeedsReceivableOnlinePayment(t *testing.T) {
	cases := map[string]struct {
		gate    keyGate
		checker *activationCheckerFake
	}{
		"not an activation invoice":                        {resellerGate(), &activationCheckerFake{invoices: map[string]bool{}}},
		"checker error fails closed":                       {resellerGate(), &activationCheckerFake{err: errors.New("db down")}},
		"activation but org lacks landing.page_activation": {keyGate{}, &activationCheckerFake{invoices: map[string]bool{"inv-1": true}}},
		"no checker configured":                            {resellerGate(), nil},
	}
	for name, tc := range cases {
		h := newOnlineHarness(t)
		h.svc.gate = tc.gate
		if tc.checker != nil {
			h.svc.WithActivationInvoices(tc.checker)
		}
		if _, _, err := h.svc.Checkout(ctx, "tok"); !errors.Is(err, ErrOnlinePaymentUnavailable) || h.doku.calls != 0 {
			t.Fatalf("%s: err=%v calls=%d, want ErrOnlinePaymentUnavailable", name, err, h.doku.calls)
		}
	}
}

func TestCheckoutWithReceivableOnlinePaymentSkipsActivationLookup(t *testing.T) {
	h := newOnlineHarness(t)
	checker := &activationCheckerFake{err: errors.New("must not be needed")}
	h.svc.gate = keyGate{"receivable.online_payment": true}
	h.svc.WithActivationInvoices(checker)
	if _, _, err := h.svc.Checkout(ctx, "tok"); err != nil {
		t.Fatalf("existing rule must be unchanged: %v", err)
	}
	if len(checker.calls) != 0 {
		t.Fatalf("checker called %v", checker.calls)
	}
}

func TestPublicCanPayActivationInvoice(t *testing.T) {
	h := newPublicHarness(domain.InvoiceIssued, false)
	h.svc.entitlements = resellerGate()
	h.svc.WithActivationInvoices(&activationCheckerFake{invoices: map[string]bool{"inv-1": true}})
	if v, err := h.svc.View(ctx, "tok1"); err != nil || !v.CanPay {
		t.Fatalf("activation invoice must be payable: %+v err=%v", v.CanPay, err)
	}

	for name, checker := range map[string]*activationCheckerFake{
		"not activation": {invoices: map[string]bool{}},
		"checker error":  {err: errors.New("db down")},
	} {
		h := newPublicHarness(domain.InvoiceIssued, false)
		h.svc.entitlements = resellerGate()
		h.svc.WithActivationInvoices(checker)
		if v, _ := h.svc.View(ctx, "tok1"); v.CanPay {
			t.Fatalf("%s: must not be payable", name)
		}
	}

	void := newPublicHarness(domain.InvoiceVoid, false)
	void.svc.entitlements = resellerGate()
	void.svc.WithActivationInvoices(&activationCheckerFake{invoices: map[string]bool{"inv-1": true}})
	if v, _ := void.svc.View(ctx, "tok1"); v.CanPay {
		t.Fatal("void activation invoice must never be payable")
	}
}

func TestOnlinePaymentAllowedRule(t *testing.T) {
	inv := domain.Invoice{ID: "inv-9"}
	act := &activationCheckerFake{invoices: map[string]bool{"inv-9": true}}
	cases := []struct {
		name    string
		gate    Entitlements
		checker ActivationInvoiceChecker
		want    bool
	}{
		{"nil gate", nil, act, false},
		{"receivable.online_payment", keyGate{"receivable.online_payment": true}, nil, true},
		{"activation + landing.page_activation", resellerGate(), act, true},
		{"activation without landing.page_activation", keyGate{}, act, false},
		{"non-activation with landing.page_activation", resellerGate(), &activationCheckerFake{}, false},
	}
	for _, tc := range cases {
		if got := onlinePaymentAllowed(ctx, tc.gate, tc.checker, scope, inv.ID); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
