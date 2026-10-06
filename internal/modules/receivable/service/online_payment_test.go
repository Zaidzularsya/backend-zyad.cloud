package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/platform/doku"
	"zyad.cloud/internal/shared/publiclink"
)

type fakeDoku struct {
	calls, statusCalls int
	last               doku.CreatePaymentRequest
	status             doku.TransactionStatus
}

func (f *fakeDoku) CreatePayment(_ context.Context, r doku.CreatePaymentRequest) (doku.Payment, error) {
	f.calls++
	f.last = r
	return doku.Payment{TokenID: "tk", PaymentURL: "https://doku.test/pay/" + string(rune('a'+f.calls)), ExpiredDate: nowWIB.Add(time.Hour)}, nil
}
func (f *fakeDoku) CheckStatus(context.Context, string) (doku.TransactionStatus, error) {
	f.statusCalls++
	return f.status, nil
}

type fakeCheckouts struct{ rows []repository.Checkout }

func (f *fakeCheckouts) FindReusable(_ context.Context, _ coretenant.Scope, invoiceID string, now time.Time) (repository.Checkout, error) {
	for i := len(f.rows) - 1; i >= 0; i-- {
		if c := f.rows[i]; c.InvoiceID == invoiceID && c.Status == "pending" && c.ExpiresAt.After(now) {
			return c, nil
		}
	}
	return repository.Checkout{}, pgx.ErrNoRows
}
func (f *fakeCheckouts) Create(_ context.Context, _ coretenant.Scope, c repository.Checkout) (repository.Checkout, error) {
	c.ID, c.Status, c.OrganizationID = "co-"+string(rune('0'+len(f.rows))), "pending", "11111111-1111-1111-1111-111111111111"
	f.rows = append(f.rows, c)
	return c, nil
}
func (f *fakeCheckouts) FindByDokuNumber(_ context.Context, n string) (repository.Checkout, error) {
	for i := len(f.rows) - 1; i >= 0; i-- {
		if f.rows[i].DokuInvoiceNumber == n {
			return f.rows[i], nil
		}
	}
	return repository.Checkout{}, pgx.ErrNoRows
}
func (f *fakeCheckouts) SetStatus(_ context.Context, id, status string) error {
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows[i].Status = status
		}
	}
	return nil
}

type tokLinks struct{ invoiceID string }

func (l tokLinks) Resolve(_ context.Context, token string) (publiclink.Link, error) {
	if token != "tok" {
		return publiclink.Link{}, publiclink.ErrLinkInvalid
	}
	return publiclink.Link{OrganizationID: "11111111-1111-1111-1111-111111111111", DocumentType: publiclink.DocumentInvoice,
		DocumentID: l.invoiceID, ExpiresAt: nowWIB.Add(24 * time.Hour)}, nil
}
func (tokLinks) Touch(context.Context, publiclink.Link) {}

type onlineHarness struct {
	svc      *OnlinePayment
	doku     *fakeDoku
	gate     *toggleGate
	invoice  domain.Invoice
	store    *fakeStore
	checkout *fakeCheckouts
	listener *recordingListener
	logs     *bytes.Buffer
}

type toggleGate struct{ allowed bool }

func (g *toggleGate) RequireFeature(context.Context, string, string) error {
	if !g.allowed {
		return errors.New("not entitled")
	}
	return nil
}

func newOnlineHarness(t *testing.T) *onlineHarness {
	t.Helper()
	ih := newInvoiceHarness(t)
	ih.store.invoices["inv-1"] = domain.Invoice{ID: "inv-1", InvoiceNumber: "INV-2026-0001", Status: domain.InvoiceIssued,
		Currency: "IDR", GrandTotal: "833000.00", AmountPaid: "0.00"}
	h := &onlineHarness{doku: &fakeDoku{}, gate: &toggleGate{allowed: true}, store: ih.store, checkout: &fakeCheckouts{},
		listener: ih.listener, logs: &bytes.Buffer{}, invoice: ih.store.invoices["inv-1"]}
	h.invoice.ID = "inv-1"
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(h.logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	h.svc = NewOnlinePayment(OnlinePaymentConfig{FrontendURL: "https://app.test/", NotificationURL: "https://api.test/webhooks/doku"},
		h.doku, h.checkout, ih.svc, ih.pay, tokLinks{invoiceID: "inv-1"}, publicScopes{}, h.gate, func() time.Time { return nowWIB })
	return h
}

func (h *onlineHarness) notify(status string) (bool, error) {
	return h.svc.HandleNotification(ctx, DokuNotification{InvoiceNumber: DokuNumberPrefix + "inv-1", Status: status,
		Amount: "833000", ProviderReference: "req-1", PaymentMethod: "VA"})
}

func TestCheckoutGateAndReuse(t *testing.T) {
	h := newOnlineHarness(t)
	h.gate.allowed = false
	if _, _, err := h.svc.Checkout(ctx, "tok"); !errors.Is(err, ErrOnlinePaymentUnavailable) || h.doku.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, h.doku.calls)
	}
	h.gate.allowed = true
	url1, _, err := h.svc.Checkout(ctx, "tok")
	url2, _, _ := h.svc.Checkout(ctx, "tok")
	if err != nil || url1 != url2 || h.doku.calls != 1 || h.doku.last.InvoiceNumber != "RCV-inv-1" || h.doku.last.Amount != 833000 {
		t.Fatalf("url1=%s url2=%s calls=%d req=%+v err=%v", url1, url2, h.doku.calls, h.doku.last, err)
	}
	if h.doku.last.CallbackURL != "https://app.test/i/tok?paid=1" || h.doku.last.NotificationURL != "https://api.test/webhooks/doku" {
		t.Fatalf("callback=%s notif=%s", h.doku.last.CallbackURL, h.doku.last.NotificationURL)
	}
}

func TestCheckoutRejectsNonPayableInvoice(t *testing.T) {
	h := newOnlineHarness(t)
	inv := h.store.invoices["inv-1"]
	inv.Status = domain.InvoiceVoid
	h.store.invoices["inv-1"] = inv
	if _, _, err := h.svc.Checkout(ctx, "tok"); !errors.Is(err, repository.ErrInvoiceNotPayable) || h.doku.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, h.doku.calls)
	}
	if _, _, err := h.svc.Checkout(ctx, "bad"); !errors.Is(err, ErrLinkInvalid) {
		t.Fatalf("bad token err=%v", err)
	}
}

func TestCheckoutUsesRemainingBalanceRoundedUp(t *testing.T) {
	h := newOnlineHarness(t)
	inv := h.store.invoices["inv-1"]
	inv.GrandTotal, inv.AmountPaid = "833000.50", "100000.00"
	h.store.invoices["inv-1"] = inv
	if _, _, err := h.svc.Checkout(ctx, "tok"); err != nil || h.doku.last.Amount != 733001 {
		t.Fatalf("amount=%d err=%v", h.doku.last.Amount, err)
	}
}

func TestNotificationIdempotentAndEdgeCases(t *testing.T) {
	t.Run("sukses dua kali", func(t *testing.T) {
		h := newOnlineHarness(t)
		if _, _, err := h.svc.Checkout(ctx, "tok"); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if ok, err := h.notify("SUCCESS"); !ok || err != nil {
				t.Fatalf("call %d: ok=%v err=%v", i, ok, err)
			}
		}
		if len(h.store.payments) != 1 || h.store.payments[0].Method != "doku" || len(h.listener.paid) != 1 || h.store.invoices["inv-1"].Status != domain.InvoicePaid {
			t.Fatalf("payments=%+v paid=%d inv=%+v", h.store.payments, len(h.listener.paid), h.store.invoices["inv-1"])
		}
		if h.checkout.rows[0].Status != "paid" {
			t.Fatalf("checkout status = %s", h.checkout.rows[0].Status)
		}
	})
	t.Run("nomor tak dikenal", func(t *testing.T) {
		h := newOnlineHarness(t)
		if ok, err := h.notify("SUCCESS"); ok || err != nil {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
	t.Run("invoice void", func(t *testing.T) {
		h := newOnlineHarness(t)
		_, _, _ = h.svc.Checkout(ctx, "tok")
		inv := h.store.invoices["inv-1"]
		inv.Status = domain.InvoiceVoid
		h.store.invoices["inv-1"] = inv
		if ok, err := h.notify("SUCCESS"); !ok || err != nil {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if len(h.store.payments) != 0 || !strings.Contains(h.logs.String(), "perlu refund") {
			t.Fatalf("payments=%d logs=%s", len(h.store.payments), h.logs.String())
		}
	})
	t.Run("sudah lunas manual", func(t *testing.T) {
		h := newOnlineHarness(t)
		_, _, _ = h.svc.Checkout(ctx, "tok")
		inv := h.store.invoices["inv-1"]
		inv.Status, inv.AmountPaid = domain.InvoicePaid, "833000.00"
		h.store.invoices["inv-1"] = inv
		if ok, err := h.notify("SUCCESS"); !ok || err != nil {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if len(h.store.payments) != 0 || !strings.Contains(h.logs.String(), "perlu refund") {
			t.Fatalf("payments=%d logs=%s", len(h.store.payments), h.logs.String())
		}
	})
	t.Run("bayar sebagian manual lalu DOKU sisa", func(t *testing.T) {
		h := newOnlineHarness(t)
		inv := h.store.invoices["inv-1"]
		inv.AmountPaid = "100000.00"
		h.store.invoices["inv-1"] = inv
		_, _, _ = h.svc.Checkout(ctx, "tok")
		n := DokuNotification{InvoiceNumber: "RCV-inv-1", Status: "SUCCESS", Amount: "733000", ProviderReference: "req-9"}
		if ok, err := h.svc.HandleNotification(ctx, n); !ok || err != nil || h.store.invoices["inv-1"].Status != domain.InvoicePaid {
			t.Fatalf("ok=%v err=%v inv=%+v", ok, err, h.store.invoices["inv-1"])
		}
	})
	t.Run("status gagal", func(t *testing.T) {
		h := newOnlineHarness(t)
		_, _, _ = h.svc.Checkout(ctx, "tok")
		if ok, err := h.notify("FAILED"); ok || err != nil || h.checkout.rows[0].Status != "pending" || len(h.store.payments) != 0 {
			t.Fatalf("ok=%v err=%v status=%s", ok, err, h.checkout.rows[0].Status)
		}
	})
}

func TestSyncStatusThrottled(t *testing.T) {
	h := newOnlineHarness(t)
	_, _, _ = h.svc.Checkout(ctx, "tok")
	h.doku.status = doku.TransactionStatus{Status: "PENDING"}
	if st, err := h.svc.SyncStatus(ctx, "tok"); err != nil || st != domain.InvoiceIssued {
		t.Fatalf("st=%s err=%v", st, err)
	}
	h.doku.status = doku.TransactionStatus{Status: "SUCCESS", Amount: "833000", OriginalRequestID: "req-1"}
	if _, err := h.svc.SyncStatus(ctx, "tok"); err != nil || h.doku.statusCalls != 1 {
		t.Fatalf("second call inside 10s must not hit DOKU: calls=%d err=%v", h.doku.statusCalls, err)
	}
	h2 := newOnlineHarness(t)
	_, _, _ = h2.svc.Checkout(ctx, "tok")
	h2.doku.status = doku.TransactionStatus{Status: "SUCCESS", Amount: "833000", OriginalRequestID: "req-1"}
	if st, err := h2.svc.SyncStatus(ctx, "tok"); err != nil || st != domain.InvoicePaid || len(h2.listener.paid) != 1 {
		t.Fatalf("st=%s err=%v paid=%d", st, err, len(h2.listener.paid))
	}
}
