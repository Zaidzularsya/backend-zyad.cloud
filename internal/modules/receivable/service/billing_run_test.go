package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/shared/pricing"
)

// recordingInvoices mencatat invoice baru yang dibuat CreateAndIssue (idempotent hit tidak dihitung).
type recordingInvoices struct {
	InvoiceService
	seen    map[string]bool
	ordered []domain.Invoice
	created int
}

func (r *recordingInvoices) CreateAndIssue(c context.Context, sc coretenant.Scope, in InvoiceInput, actor string) (domain.Invoice, error) {
	inv, err := r.InvoiceService.CreateAndIssue(c, sc, in, actor)
	if err == nil && !r.seen[inv.ID] {
		r.seen[inv.ID] = true
		r.ordered = append(r.ordered, inv)
		r.created++
	}
	return inv, err
}

func (r *recordingInvoices) last() domain.Invoice { return r.ordered[len(r.ordered)-1] }

func (r *recordingInvoices) keys() string {
	ks := make([]string, len(r.ordered))
	for i, inv := range r.ordered {
		ks[i] = inv.IdempotencyKey
	}
	return strings.Join(ks, ",")
}

type runHarness struct {
	t         *testing.T
	run       *BillingRun
	contracts *fakeContracts
	invoices  *recordingInvoices
	settings  *fakeSettings
	store     *fakeStore
	listener  *recordingListener
}

func newRunHarness(t *testing.T, nowUTC string) *runHarness {
	t.Helper()
	now, err := time.Parse(time.RFC3339, nowUTC)
	if err != nil {
		t.Fatal(err)
	}
	ih := newInvoiceHarness(t)
	registry := NewRegistry()
	registry.Add(ih.listener)
	h := &runHarness{
		t: t, contracts: newFakeContracts(), store: ih.store, listener: ih.listener,
		invoices: &recordingInvoices{InvoiceService: ih.svc, seen: map[string]bool{}},
		settings: &fakeSettings{domain.Settings{InvoiceLeadDays: 7, PaymentTermsDays: 7, DefaultChannels: []string{"email"}}},
	}
	h.run = NewBillingRun(h.contracts, h.invoices, h.settings, registry, func() time.Time { return now })
	return h
}

func item(id string, f pricing.Frequency, timing pricing.PaymentTiming, idx int) domain.ContractItem {
	return domain.ContractItem{ID: id, Description: "Internet", Quantity: "1", UnitPrice: "300000", TaxPercent: "0",
		Frequency: f, PaymentTiming: timing, PeriodIndex: idx}
}

// contract menyimpan contract aktif; periode item dihitung dari tanggal mulai seperti BillOrder.
func (h *runHarness) contract(id, start string, items ...domain.ContractItem) *domain.Contract {
	c := domain.Contract{ID: id, AccountID: "a1", ContractNumber: "CTR-" + id, Status: domain.ContractActive,
		StartDate: date(start), Channels: []string{"email"}, Currency: "IDR"}
	for i := range items {
		items[i].NextPeriodStart, items[i].NextPeriodEnd = pricing.PeriodRange(c.StartDate, items[i].Frequency, items[i].PeriodIndex)
		items[i].Position = i
	}
	c.Items = items
	h.contracts.byID[id] = c
	return &c
}

func TestBillingRunPrepaidLeadAndIdempotent(t *testing.T) {
	h := newRunHarness(t, "2026-10-29T03:00:00Z") // 10:00 WIB, lead 7
	h.contract("c1", "2026-10-05", item("A", pricing.Monthly, pricing.Prepaid, 1))
	res, err := h.run.Run(ctx, scope)
	if err != nil || res.Invoices != 1 || res.Advanced != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	inv := h.invoices.last()
	if inv.IdempotencyKey != "period:2026-10-29" || inv.Items[0].PeriodStart.Format("2006-01-02") != "2026-11-05" ||
		inv.Items[0].PeriodEnd.Format("2006-01-02") != "2026-12-04" || inv.Items[0].Description != "Internet (periode 5 Nov 2026 – 4 Des 2026)" {
		t.Fatalf("inv=%+v", inv)
	}
	if inv.ContractID != "c1" || inv.SourceID != "c1" || inv.SourceType != domain.SourceContract || inv.Items[0].ContractItemID != "A" {
		t.Fatalf("contract linkage = %+v", inv)
	}
	res2, _ := h.run.Run(ctx, scope)
	if res2.Invoices != 0 || h.invoices.created != 1 {
		t.Fatal("second run same day must not bill again")
	}
}

func TestBillingRunCatchUpOneInvoicePerPeriod(t *testing.T) {
	h := newRunHarness(t, "2027-01-30T03:00:00Z")
	h.contract("c1", "2026-10-05", item("A", pricing.Monthly, pricing.Prepaid, 1)) // periode 2..5 jatuh tempo (Nov, Des, Jan, Feb[H-7 = 29 Jan])
	res, _ := h.run.Run(ctx, scope)
	if res.Invoices != 4 || h.invoices.keys() != "period:2026-10-29,period:2026-11-28,period:2026-12-29,period:2027-01-29" {
		t.Fatalf("res=%+v keys=%s", res, h.invoices.keys())
	}
}

func TestBillingRunGroupsSameBillDateAndPostpaid(t *testing.T) {
	h := newRunHarness(t, "2026-11-05T03:00:00Z")
	h.contract("c1", "2026-10-05",
		item("A", pricing.Monthly, pricing.Prepaid, 1), item("B", pricing.Monthly, pricing.Prepaid, 1),
		item("C", pricing.Monthly, pricing.Postpaid, 0))
	res, _ := h.run.Run(ctx, scope)
	if res.Invoices != 2 || res.Advanced != 3 || h.invoices.keys() != "period:2026-10-29,period:2026-11-05" {
		t.Fatalf("res=%+v keys=%s", res, h.invoices.keys())
	}
	if n := len(h.invoices.ordered[0].Items); n != 2 {
		t.Fatalf("prepaid invoice has %d lines, want 2", n)
	}
	post := h.invoices.ordered[1]
	if len(post.Items) != 1 || post.Items[0].PeriodStart.Format("2006-01-02") != "2026-10-05" ||
		post.Items[0].PeriodEnd.Format("2006-01-02") != "2026-11-04" {
		t.Fatalf("postpaid invoice = %+v", post)
	}
}

func TestBillingRunWIBMidnight(t *testing.T) {
	h := newRunHarness(t, "2026-10-28T17:30:00Z") // = 29 Okt 00:30 WIB
	h.contract("c1", "2026-10-05", item("A", pricing.Monthly, pricing.Prepaid, 1))
	res, _ := h.run.Run(ctx, scope)
	if res.Invoices != 1 || h.invoices.keys() != "period:2026-10-29" {
		t.Fatalf("res=%+v keys=%s", res, h.invoices.keys())
	}
}

func TestBillingRunResumesAfterAdvanceFailure(t *testing.T) {
	h := newRunHarness(t, "2026-10-29T03:00:00Z")
	h.contract("c1", "2026-10-05", item("A", pricing.Monthly, pricing.Prepaid, 1))
	h.contracts.failAdvanceOnce = true
	res, err := h.run.Run(ctx, scope)
	if err != nil || len(res.Errors) != 1 || res.Advanced != 0 || h.invoices.created != 1 {
		t.Fatalf("run1 res=%+v err=%v created=%d", res, err, h.invoices.created)
	}
	res, _ = h.run.Run(ctx, scope)
	if len(res.Errors) != 0 || res.Advanced != 1 || h.invoices.created != 1 {
		t.Fatalf("run2 res=%+v created=%d (invoice must be reused, not duplicated)", res, h.invoices.created)
	}
}

func TestBillingRunLeadDaysChange(t *testing.T) {
	h := newRunHarness(t, "2026-10-23T03:00:00Z")
	h.contract("c1", "2026-10-05", item("A", pricing.Monthly, pricing.Prepaid, 1))
	if res, _ := h.run.Run(ctx, scope); res.Invoices != 0 {
		t.Fatalf("lead 7 on 23 Okt must not bill: %+v", res)
	}
	h.settings.s.InvoiceLeadDays = 14
	res, _ := h.run.Run(ctx, scope)
	if res.Invoices != 1 || h.invoices.keys() != "period:2026-10-22" {
		t.Fatalf("lead 14: res=%+v keys=%s", res, h.invoices.keys())
	}
}

func TestBillingRunEndsExpiredContracts(t *testing.T) {
	h := newRunHarness(t, "2026-11-01T03:00:00Z")
	c := h.contract("c1", "2026-10-05", item("A", pricing.Monthly, pricing.Prepaid, 1))
	end := date("2026-10-31")
	c.EndDate = &end
	h.contracts.byID["c1"] = *c
	res, _ := h.run.Run(ctx, scope)
	if res.Invoices != 0 || res.Ended != 1 || h.contracts.byID["c1"].Status != domain.ContractEnded {
		t.Fatalf("res=%+v status=%s", res, h.contracts.byID["c1"].Status)
	}
	if got := h.listener.ended; len(got) != 1 || got[0].ID != "c1" {
		t.Fatalf("ContractEnded events = %+v, want one for c1", got)
	}
	h.run.Run(ctx, scope)
	if len(h.listener.ended) != 1 {
		t.Fatalf("second run re-emitted ContractEnded: %+v", h.listener.ended)
	}
}

func TestBillingRunFailedGroupDoesNotBlockOthers(t *testing.T) {
	h := newRunHarness(t, "2026-10-29T03:00:00Z")
	h.contract("c1", "2026-10-05", item("A", pricing.Monthly, pricing.Prepaid, 1))
	bad := h.contract("c2", "2026-10-05", item("B", pricing.Monthly, pricing.Prepaid, 1))
	bad.AccountID = "" // CreateAndIssue menolak: account wajib
	h.contracts.byID["c2"] = *bad
	res, err := h.run.Run(ctx, scope)
	if err != nil || res.Invoices != 1 || len(res.Errors) != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

type billingScopes struct{ failFor map[string]bool }

func (s billingScopes) ResolveWorkerOrganization(c context.Context, orgID, identity string) (coretenant.Context, error) {
	if identity != BillingRunWorkerIdentity {
		return coretenant.Context{}, errors.New("wrong identity " + identity)
	}
	return runnerScopes{failFor: s.failFor}.resolve(orgID)
}

func TestBillingRunnerVisitsEveryOrganizationAndKeepsGoingAfterFailure(t *testing.T) {
	h := newRunHarness(t, "2026-10-29T03:00:00Z")
	h.contract("c1", "2026-10-05", item("A", pricing.Monthly, pricing.Prepaid, 1))
	bad := "22222222-2222-2222-2222-222222222222"
	orgs := &fakeOrgs{ids: []string{"11111111-1111-1111-1111-111111111111", bad, "33333333-3333-3333-3333-333333333333"}}
	r := NewBillingRunner(orgs, billingScopes{failFor: map[string]bool{bad: true}}, h.run)
	r.pageSize = 2
	res, err := r.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// fake contract store tidak memisahkan organisasi: org pertama menagih, org ketiga tidak menemukan apa-apa.
	if res.Checked != 3 || res.Failed != 1 || res.Invoices != 1 || res.Advanced != 1 {
		t.Fatalf("result = %+v", res)
	}
	if len(orgs.calls) != 2 || orgs.calls[1] != [2]int{2, 2} {
		t.Fatalf("paging calls = %v", orgs.calls)
	}
}

func TestBillingRunnerStopsWhenContextIsCancelled(t *testing.T) {
	h := newRunHarness(t, "2026-10-29T03:00:00Z")
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	r := NewBillingRunner(&fakeOrgs{ids: []string{"11111111-1111-1111-1111-111111111111"}}, billingScopes{}, h.run)
	if _, err := r.RunOnce(cctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
