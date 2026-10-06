package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/pricing"
)

type wonItem struct {
	attrs    pricing.Attributes
	delivery domain.DeliveryStatus
}

type wonOrders struct {
	repository.SalesOrderRepository
	orders []domain.SalesOrder
}

func (o *wonOrders) ListByDeal(context.Context, coretenant.Scope, string) ([]domain.SalesOrder, error) {
	return o.orders, nil
}
func (o *wonOrders) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.SalesOrder, error) {
	return o.orders[0], nil
}

type wonDeals struct {
	status    domain.DealStatus
	closed    int
	panicOnce bool
}

func (d *wonDeals) Get(context.Context, coretenant.Scope, string) (domain.Deal, error) {
	return domain.Deal{Status: d.status}, nil
}
func (d *wonDeals) CloseWon(context.Context, coretenant.Scope, string, string) (domain.Deal, error) {
	if d.panicOnce {
		d.panicOnce = false
		panic("boom")
	}
	d.closed++
	d.status = domain.DealStatusWon
	return domain.Deal{}, nil
}

type wonInvoices struct{ status string }

func (w wonInvoices) InvoiceStatus(context.Context, coretenant.Scope, string) (string, string, error) {
	return w.status, "INV-2026-0001", nil
}

type wonContracts struct{}

func (wonContracts) ContractNumber(context.Context, coretenant.Scope, string) (string, error) {
	return "CTR-2026-0001", nil
}

type wonHarness struct {
	eval   *WonEvaluator
	deals  *wonDeals
	orders *wonOrders
}

func prepaidOneTime() pricing.Attributes {
	return pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Prepaid}
}
func prepaidRecurring() pricing.Attributes {
	return pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}
}
func postpaidRecurring() pricing.Attributes {
	return pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Postpaid}
}
func postpaidOneTime() pricing.Attributes {
	return pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Postpaid}
}

func newWonHarness(items []wonItem, invoiceStatus string, withContract bool) *wonHarness {
	so := domain.SalesOrder{ID: "so1", SONumber: "SO-2026-0001", DealID: soPtr("d1"), Status: domain.SalesOrderConfirmed, InitialInvoiceID: "inv1"}
	if withContract {
		so.ContractID = "ctr1"
	}
	for i, it := range items {
		d := it.delivery
		if d == "" {
			d = domain.DeliveryNotApplicable
		}
		so.Items = append(so.Items, domain.SalesOrderItem{ID: string(rune('a' + i)), Description: "item", Pricing: it.attrs, DeliveryStatus: d})
	}
	h := &wonHarness{deals: &wonDeals{status: domain.DealStatusOpen}, orders: &wonOrders{orders: []domain.SalesOrder{so}}}
	h.eval = NewWonEvaluator(h.orders, h.deals, wonInvoices{invoiceStatus}, wonContracts{}, nil)
	return h
}

func TestWonEvaluator(t *testing.T) {
	cases := []struct {
		name     string
		items    []wonItem
		invoice  string
		contract bool
		wantWon  bool
	}{
		{"isp belum bayar", []wonItem{{attrs: prepaidOneTime()}, {attrs: prepaidRecurring()}}, "issued", true, false},
		{"isp lunas", []wonItem{{attrs: prepaidOneTime()}, {attrs: prepaidRecurring()}}, "paid", true, true},
		{"proyek: kontrak ada, website belum diterima", []wonItem{{attrs: postpaidRecurring()}, {attrs: postpaidOneTime(), delivery: domain.DeliveryPending}}, "", true, false},
		{"proyek: kontrak ada, website diterima", []wonItem{{attrs: postpaidRecurring()}, {attrs: postpaidOneTime(), delivery: domain.DeliveryDelivered}}, "", true, true},
		{"campuran: prabayar lunas, pascabayar sekali belum diterima", []wonItem{{attrs: prepaidOneTime()}, {attrs: postpaidOneTime(), delivery: domain.DeliveryPending}}, "paid", false, false},
		{"pascabayar berulang tanpa kontrak", []wonItem{{attrs: postpaidRecurring()}}, "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newWonHarness(c.items, c.invoice, c.contract)
			won, err := h.eval.Evaluate(soCtx, soScope, "d1")
			if err != nil || won != c.wantWon || (c.wantWon && h.deals.closed != 1) {
				t.Fatalf("won=%v closed=%d err=%v", won, h.deals.closed, err)
			}
		})
	}
}

func TestEvaluateSkipsClosedDealsAndIsIdempotent(t *testing.T) {
	items := []wonItem{{attrs: prepaidOneTime()}}
	for _, status := range []domain.DealStatus{domain.DealStatusWon, domain.DealStatusLost} {
		h := newWonHarness(items, "paid", false)
		h.deals.status = status
		if won, _ := h.eval.Evaluate(soCtx, soScope, "d1"); won || h.deals.closed != 0 {
			t.Fatalf("status %s must not be touched", status)
		}
	}
	h := newWonHarness(items, "paid", false)
	_, _ = h.eval.Evaluate(soCtx, soScope, "d1")
	_, _ = h.eval.Evaluate(soCtx, soScope, "d1")
	if h.deals.closed != 1 {
		t.Fatalf("closed = %d", h.deals.closed)
	}
	empty := newWonHarness(nil, "paid", false)
	if won, _ := empty.eval.Evaluate(soCtx, soScope, "d1"); won {
		t.Fatal("SO without lines must not trigger Won")
	}
}

func TestChecklistLabels(t *testing.T) {
	h := newWonHarness([]wonItem{{attrs: prepaidOneTime()}, {attrs: postpaidRecurring()}}, "issued", true)
	lists, err := h.eval.Checklist(soCtx, soScope, "d1")
	if err != nil || len(lists) != 1 {
		t.Fatalf("lists=%v err=%v", lists, err)
	}
	c := lists[0].Conditions
	if c[0].Met || c[0].Label != "Menunggu pembayaran INV-2026-0001" || !c[1].Met || c[1].Label != "Kontrak CTR-2026-0001 terbentuk" {
		t.Fatalf("conditions=%+v", c)
	}
}

func TestListenerSurvivesPanicAndChecklistRecovers(t *testing.T) {
	h := newWonHarness([]wonItem{{attrs: prepaidOneTime()}}, "paid", false)
	h.deals.panicOnce = true
	l := NewReceivableListener(h.orders, h.eval, nil)
	l.InvoicePaid(soCtx, soScope, receivableservice.InvoiceRef{ID: "inv1", Number: "INV-2026-0001", SourceType: receivabledomain.SourceSalesOrder, SourceID: "so1"})
	if h.deals.closed != 0 {
		t.Fatal("first CloseWon panicked")
	}
	// Deal Detail membuka won-checklist → Evaluate ulang memulihkan.
	if _, err := h.eval.Checklist(soCtx, soScope, "d1"); err != nil {
		t.Fatal(err)
	}
	if won, err := h.eval.Evaluate(soCtx, soScope, "d1"); err != nil || !won || h.deals.closed != 1 {
		t.Fatalf("won=%v closed=%d err=%v", won, h.deals.closed, err)
	}
	_ = errors.New
}
