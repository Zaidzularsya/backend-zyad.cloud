package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/shared/pricing"
)

// ---- fakes khusus order billing ----

type fakeContracts struct {
	byID            map[string]domain.Contract
	created         int
	failAdvanceOnce bool
}

func newFakeContracts() *fakeContracts { return &fakeContracts{byID: map[string]domain.Contract{}} }

func (f *fakeContracts) Create(_ context.Context, _ coretenant.Scope, p repository.CreateContractParams) (domain.Contract, error) {
	for _, c := range f.byID {
		if string(c.SourceType) == p.SourceType && c.SourceID == p.SourceID && p.SourceID != "" {
			return domain.Contract{}, repository.ErrDuplicateSource
		}
	}
	f.created++
	c := domain.Contract{
		ID: fmt.Sprintf("ctr-%d", f.created), AccountID: p.AccountID, ContractNumber: p.Number, Status: domain.ContractActive,
		SourceType: domain.SourceType(p.SourceType), SourceID: p.SourceID, StartDate: p.StartDate, Channels: p.Channels,
	}
	for i, it := range p.Items {
		c.Items = append(c.Items, domain.ContractItem{
			ID: fmt.Sprintf("ci-%d-%d", f.created, i), Description: it.Description, Frequency: it.Frequency,
			PaymentTiming: it.PaymentTiming, PeriodIndex: it.PeriodIndex, NextPeriodStart: it.NextPeriodStart,
			NextPeriodEnd: it.NextPeriodEnd, SourceLineID: it.SourceLineID, Position: i, Features: it.Features,
		})
	}
	f.byID[c.ID] = c
	return c, nil
}
func (f *fakeContracts) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Contract, error) {
	c, ok := f.byID[id]
	if !ok {
		return domain.Contract{}, pgx.ErrNoRows
	}
	return c, nil
}
func (f *fakeContracts) FindBySource(_ context.Context, _ coretenant.Scope, st, id string) (domain.Contract, error) {
	for _, c := range f.byID {
		if string(c.SourceType) == st && c.SourceID == id {
			return c, nil
		}
	}
	return domain.Contract{}, pgx.ErrNoRows
}
func (f *fakeContracts) List(context.Context, coretenant.Scope, repository.ContractListFilter) ([]domain.Contract, int64, error) {
	return nil, 0, nil
}
func (f *fakeContracts) SetEndDate(_ context.Context, _ coretenant.Scope, id string, end *time.Time, _ string) (domain.Contract, error) {
	c, ok := f.byID[id]
	if !ok {
		return domain.Contract{}, pgx.ErrNoRows
	}
	if c.Status != domain.ContractActive {
		return domain.Contract{}, repository.ErrContractNotActive
	}
	c.EndDate = end
	f.byID[id] = c
	return c, nil
}
func (f *fakeContracts) End(_ context.Context, _ coretenant.Scope, id string, end time.Time, reason, _ string) (domain.Contract, error) {
	c, ok := f.byID[id]
	if !ok {
		return domain.Contract{}, pgx.ErrNoRows
	}
	if c.Status != domain.ContractActive {
		return domain.Contract{}, repository.ErrContractNotActive
	}
	c.Status, c.EndDate, c.EndReason = domain.ContractEnded, &end, reason
	f.byID[id] = c
	return c, nil
}

// DueItems meniru query repository: rumus BillOn, batas end_date, urut contract_id lalu next_period_start.
func (f *fakeContracts) DueItems(_ context.Context, _ coretenant.Scope, today time.Time, lead, limit int) ([]repository.DueItem, error) {
	out := []repository.DueItem{}
	for _, c := range f.byID {
		if c.Status != domain.ContractActive {
			continue
		}
		for _, it := range c.Items {
			billOn := it.NextPeriodEnd.AddDate(0, 0, 1)
			if it.PaymentTiming == pricing.Prepaid {
				billOn = it.NextPeriodStart.AddDate(0, 0, -lead)
			}
			if billOn.After(today) || (c.EndDate != nil && it.NextPeriodStart.After(*c.EndDate)) {
				continue
			}
			header := c
			header.Items = nil
			out = append(out, repository.DueItem{ContractID: c.ID, ContractNumber: c.ContractNumber, AccountID: c.AccountID,
				ItemID: it.ID, Contract: header, Item: it, BillOn: billOn})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ContractID != out[j].ContractID {
			return out[i].ContractID < out[j].ContractID
		}
		return out[i].Item.NextPeriodStart.Before(out[j].Item.NextPeriodStart)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeContracts) Advance(_ context.Context, _ coretenant.Scope, itemID string, from int, nextStart, nextEnd time.Time) (bool, error) {
	if f.failAdvanceOnce {
		f.failAdvanceOnce = false
		return false, errors.New("db down")
	}
	for _, c := range f.byID {
		for i, it := range c.Items {
			if it.ID == itemID {
				if it.PeriodIndex != from {
					return false, nil
				}
				c.Items[i].PeriodIndex, c.Items[i].NextPeriodStart, c.Items[i].NextPeriodEnd = from+1, nextStart, nextEnd
				return true, nil
			}
		}
	}
	return false, nil
}

func (f *fakeContracts) EndExpired(_ context.Context, _ coretenant.Scope, today time.Time) (int64, error) {
	var n int64
	for id, c := range f.byID {
		if c.Status != domain.ContractActive || c.EndDate == nil || !c.EndDate.Before(today) {
			continue
		}
		open := false
		for _, it := range c.Items {
			open = open || !it.NextPeriodStart.After(*c.EndDate)
		}
		if !open {
			c.Status, c.EndReason = domain.ContractEnded, "Masa kontrak berakhir"
			f.byID[id] = c
			n++
		}
	}
	return n, nil
}

// failingInvoices membungkus InvoiceService agar CreateAndIssue bisa digagalkan sekali.
type failingInvoices struct {
	InvoiceService
	failNext bool
}

func (f *failingInvoices) CreateAndIssue(c context.Context, sc coretenant.Scope, in InvoiceInput, actor string) (domain.Invoice, error) {
	if f.failNext {
		f.failNext = false
		return domain.Invoice{}, errors.New("asset store down")
	}
	return f.InvoiceService.CreateAndIssue(c, sc, in, actor)
}

type orderHarness struct {
	billing   OrderBilling
	store     *fakeStore
	contracts *fakeContracts
	invoices  *failingInvoices
	listener  *recordingListener
	counter   *fakeCounter
}

func newOrderHarness(t *testing.T) *orderHarness {
	t.Helper()
	ih := newInvoiceHarness(t)
	h := &orderHarness{store: ih.store, contracts: newFakeContracts(), invoices: &failingInvoices{InvoiceService: ih.svc}, listener: ih.listener, counter: ih.counter}
	registry := NewRegistry()
	registry.Add(ih.listener)
	ob := NewOrderBilling(fakeAccounts{}, h.invoices, h.contracts, ih.counter, registry).(*orderBilling)
	ob.now = func() time.Time { return nowWIB }
	h.billing = ob
	return h
}

func date(s string) time.Time { return *day(s) }

func monthlyPrepaid() pricing.Attributes {
	return pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}
}

func ispRequest(soID string, start time.Time) OrderBillingRequest {
	return OrderBillingRequest{
		SourceType: domain.SourceSalesOrder, SourceID: soID, StartDate: start, Channels: []string{"email"}, ActorUserID: "u1",
		Account: AccountInput{Name: "Budi", Email: "budi@example.com", SourceType: "crm_contact", SourceID: "c1"},
		Lines: []OrderLine{
			{SourceLineID: "l1", Description: "Instalasi", Quantity: "1", UnitPrice: "500000"},
			{SourceLineID: "l2", Description: "Internet", Quantity: "1", UnitPrice: "300000", TaxPercent: "11", Attrs: monthlyPrepaid()},
		},
	}
}

func postpaidRequest(soID string) OrderBillingRequest {
	return OrderBillingRequest{
		SourceType: domain.SourceSalesOrder, SourceID: soID, StartDate: date("2026-10-05"), ActorUserID: "u1",
		Account: AccountInput{Name: "Budi", SourceType: "crm_contact", SourceID: "c1"},
		Lines: []OrderLine{
			{SourceLineID: "l1", Description: "Website", UnitPrice: "5000000", Attrs: pricing.Attributes{PaymentTiming: pricing.Postpaid}},
			{SourceLineID: "l2", Description: "Maintenance", UnitPrice: "200000",
				Attrs: pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Postpaid}},
		},
	}
}

func TestBillOrderISPCase(t *testing.T) {
	h := newOrderHarness(t)
	req := ispRequest("so1", date("2026-10-05"))
	res, err := h.billing.BillOrder(ctx, scope, req)
	if err != nil || res.AccountID == "" || res.InitialInvoiceID == "" || res.ContractID == "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	inv := h.store.invoices[res.InitialInvoiceID]
	if inv.GrandTotal != "833000.00" || inv.Status != domain.InvoiceIssued || len(inv.Items) != 2 {
		t.Fatalf("initial=%+v", inv)
	}
	if inv.ContractID != res.ContractID || inv.PeriodStart == nil || inv.PeriodStart.Format("2006-01-02") != "2026-10-05" || inv.PeriodEnd.Format("2006-01-02") != "2026-11-04" {
		t.Fatalf("invoice contract/period = %q %v %v", inv.ContractID, inv.PeriodStart, inv.PeriodEnd)
	}
	if inv.Items[1].Description != "Internet (periode 5 Okt 2026 – 4 Nov 2026)" {
		t.Fatalf("description = %q", inv.Items[1].Description)
	}
	ci := h.contracts.byID[res.ContractID].Items[0]
	if ci.PeriodIndex != 1 || ci.NextPeriodStart.Format("2006-01-02") != "2026-11-05" || ci.NextPeriodEnd.Format("2006-01-02") != "2026-12-04" || inv.Items[1].ContractItemID != ci.ID {
		t.Fatalf("contract item=%+v", ci)
	}
	again, _ := h.billing.BillOrder(ctx, scope, req)
	if again != res || len(h.store.invoices) != 1 || h.contracts.created != 1 || len(h.listener.contracts) != 1 {
		t.Fatal("BillOrder must be idempotent")
	}
}

func TestBillOrderStoresItemFeatures(t *testing.T) {
	h := newOrderHarness(t)
	req := ispRequest("so-feat", date("2026-10-05"))
	features := json.RawMessage(`[{"feature_key":"user.max","value":5,"label":"5 user"}]`)
	req.Lines[1].Features = features
	res, err := h.billing.BillOrder(ctx, scope, req)
	if err != nil || res.ContractID == "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	items := h.contracts.byID[res.ContractID].Items
	if len(items) != 1 || string(items[0].Features) != string(features) {
		t.Fatalf("contract item features = %s", items[0].Features)
	}
}

func TestBillOrderPostpaidOnly(t *testing.T) {
	h := newOrderHarness(t)
	res, err := h.billing.BillOrder(ctx, scope, postpaidRequest("so2"))
	if err != nil || res.InitialInvoiceID != "" || res.ContractID == "" || len(h.store.invoices) != 0 {
		t.Fatalf("res=%+v err=%v invoices=%d", res, err, len(h.store.invoices))
	}
	ci := h.contracts.byID[res.ContractID].Items[0]
	if ci.PeriodIndex != 0 || ci.NextPeriodStart.Format("2006-01-02") != "2026-10-05" || ci.NextPeriodEnd.Format("2006-01-02") != "2026-11-04" {
		t.Fatalf("contract item=%+v", ci)
	}
}

func TestBillOrderResumesAfterFailure(t *testing.T) {
	h := newOrderHarness(t)
	h.invoices.failNext = true
	req := ispRequest("so3", date("2026-10-05"))
	if _, err := h.billing.BillOrder(ctx, scope, req); err == nil {
		t.Fatal("first attempt must fail")
	}
	if h.contracts.created != 1 || len(h.store.invoices) != 0 {
		t.Fatalf("contract=%d invoices=%d after failure", h.contracts.created, len(h.store.invoices))
	}
	res, err := h.billing.BillOrder(ctx, scope, req)
	if err != nil || res.InitialInvoiceID == "" || h.contracts.created != 1 || len(h.store.invoices) != 1 {
		t.Fatalf("res=%+v err=%v contracts=%d invoices=%d", res, err, h.contracts.created, len(h.store.invoices))
	}
	if len(h.listener.contracts) != 1 {
		t.Fatalf("ContractCreated fired %d times", len(h.listener.contracts))
	}
}

func TestBillDeliveryIdempotentPerBatch(t *testing.T) {
	h := newOrderHarness(t)
	req := postpaidRequest("so4")
	lines := req.Lines[:1]
	first, err := h.billing.BillDelivery(ctx, scope, domain.SourceSalesOrder, "so4", "b1", lines, req)
	if err != nil || first == "" {
		t.Fatalf("first=%q err=%v", first, err)
	}
	again, _ := h.billing.BillDelivery(ctx, scope, domain.SourceSalesOrder, "so4", "b1", lines, req)
	if again != first || len(h.store.invoices) != 1 {
		t.Fatalf("again=%q invoices=%d", again, len(h.store.invoices))
	}
	other, _ := h.billing.BillDelivery(ctx, scope, domain.SourceSalesOrder, "so4", "b2", lines, req)
	if other == first || len(h.store.invoices) != 2 {
		t.Fatalf("other=%q invoices=%d", other, len(h.store.invoices))
	}
}

func contractParamsForTest() repository.CreateContractParams {
	return repository.CreateContractParams{
		Number: "CTR-2026-0001", AccountID: "a1", SourceType: "sales_order", SourceID: "so9", StartDate: date("2026-10-05"),
		Items: []repository.ContractItemParams{{Description: "Internet", Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid,
			NextPeriodStart: date("2026-11-05"), NextPeriodEnd: date("2026-12-04")}},
	}
}
