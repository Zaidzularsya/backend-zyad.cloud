package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/pricing"
)

var (
	soCtx   = context.Background()
	soScope = coretenant.Scope{}
	// 2026-10-05 10:00 WIB
	soNow = time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC)
)

func soPtr(s string) *string { return &s }

func soDate(s string) *time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return &d
}

type soRepoFake struct {
	byID map[string]domain.SalesOrder
	seq  int
}

func (r *soRepoFake) CreateFromQuotation(_ context.Context, _ coretenant.Scope, q domain.Quotation, number string, d repository.SalesOrderDraftFields, by string) (domain.SalesOrder, bool, error) {
	for _, so := range r.byID {
		if so.QuotationID == q.ID {
			return so, false, nil
		}
	}
	r.seq++
	so := domain.SalesOrder{ID: fmt.Sprintf("so-%d", r.seq), SONumber: number, QuotationID: q.ID, QuotationNumber: q.QuotationNumber,
		DealID: q.DealID, ContactID: q.ContactID, Status: domain.SalesOrderDraft, BillingStatus: domain.BillingNone,
		BillToName: d.BillToName, BillToEmail: d.BillToEmail, BillToPhone: d.BillToPhone, Channels: d.Channels, PICUserID: d.PICUserID, Currency: "IDR"}
	for i, it := range q.Items {
		si := domain.SalesOrderItem{ID: fmt.Sprintf("it-%d-%d", r.seq, i), Description: it.Description, Quantity: it.Quantity, UnitPrice: it.UnitPrice,
			Pricing: it.Pricing, DeliveryStatus: domain.DeliveryNotApplicable}
		if it.Pricing.ChargeType == pricing.OneTime && it.Pricing.PaymentTiming == pricing.Postpaid {
			si.DeliveryStatus = domain.DeliveryPending
		}
		so.Items = append(so.Items, si)
	}
	r.byID[so.ID] = so
	return so, true, nil
}
func (r *soRepoFake) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.SalesOrder, error) {
	so, ok := r.byID[id]
	if !ok {
		return domain.SalesOrder{}, pgx.ErrNoRows
	}
	return so, nil
}
func (r *soRepoFake) FindByQuotation(_ context.Context, _ coretenant.Scope, qid string) (domain.SalesOrder, error) {
	for _, so := range r.byID {
		if so.QuotationID == qid {
			return so, nil
		}
	}
	return domain.SalesOrder{}, pgx.ErrNoRows
}
func (r *soRepoFake) ListByDeal(context.Context, coretenant.Scope, string) ([]domain.SalesOrder, error) {
	return nil, nil
}
func (r *soRepoFake) List(context.Context, coretenant.Scope, repository.SalesOrderListFilter) ([]domain.SalesOrder, int64, error) {
	return nil, 0, nil
}
func (r *soRepoFake) UpdateDraft(_ context.Context, _ coretenant.Scope, id string, f repository.SalesOrderDraftFields, _ string) (domain.SalesOrder, error) {
	so, ok := r.byID[id]
	if !ok {
		return domain.SalesOrder{}, pgx.ErrNoRows
	}
	if so.Status != domain.SalesOrderDraft {
		return domain.SalesOrder{}, repository.ErrSalesOrderNotDraft
	}
	so.StartDate, so.BillToName, so.BillToCompany, so.BillToEmail, so.BillToPhone, so.Channels, so.PICUserID =
		f.StartDate, f.BillToName, f.BillToCompany, f.BillToEmail, f.BillToPhone, f.Channels, f.PICUserID
	r.byID[id] = so
	return so, nil
}
func (r *soRepoFake) MarkConfirmed(_ context.Context, _ coretenant.Scope, id, _ string) (domain.SalesOrder, error) {
	so, ok := r.byID[id]
	if !ok {
		return domain.SalesOrder{}, pgx.ErrNoRows
	}
	if so.Status != domain.SalesOrderDraft {
		return domain.SalesOrder{}, repository.ErrSalesOrderNotDraft
	}
	so.Status, so.BillingStatus = domain.SalesOrderConfirmed, domain.BillingPending
	r.byID[id] = so
	return so, nil
}
func (r *soRepoFake) SetBillingResult(_ context.Context, _ coretenant.Scope, id string, st domain.BillingStatus, msg, acc, inv, ctr string) (domain.SalesOrder, error) {
	so := r.byID[id]
	so.BillingStatus, so.BillingError = st, msg
	if acc != "" {
		so.ReceivableAccountID = acc
	}
	if inv != "" {
		so.InitialInvoiceID = inv
	}
	if ctr != "" {
		so.ContractID = ctr
	}
	r.byID[id] = so
	return so, nil
}
func (r *soRepoFake) MarkDelivered(_ context.Context, _ coretenant.Scope, id string, ids []string, _ time.Time, note, invoiceID, _ string) (domain.SalesOrder, error) {
	so := r.byID[id]
	for _, want := range ids {
		found := false
		for i := range so.Items {
			if so.Items[i].ID == want && so.Items[i].DeliveryStatus == domain.DeliveryPending {
				so.Items[i].DeliveryStatus, so.Items[i].InvoiceID, so.Items[i].DeliveryNote = domain.DeliveryDelivered, invoiceID, note
				found = true
			}
		}
		if !found {
			return domain.SalesOrder{}, repository.ErrDeliveryNotPending
		}
	}
	r.byID[id] = so
	return so, nil
}
func (r *soRepoFake) Cancel(context.Context, coretenant.Scope, string, string) (domain.SalesOrder, error) {
	return domain.SalesOrder{}, nil
}

type soBillingFake struct {
	req           receivableservice.OrderBillingRequest
	err           error
	calls         int
	deliveryBatch []string
}

func (b *soBillingFake) BillOrder(_ context.Context, _ coretenant.Scope, req receivableservice.OrderBillingRequest) (receivableservice.OrderBillingResult, error) {
	b.calls++
	b.req = req
	if b.err != nil {
		return receivableservice.OrderBillingResult{}, b.err
	}
	return receivableservice.OrderBillingResult{AccountID: "acc1", InitialInvoiceID: "inv1", ContractID: "ctr1"}, nil
}
func (b *soBillingFake) BillDelivery(_ context.Context, _ coretenant.Scope, _ receivabledomain.SourceType, _, batch string, _ []receivableservice.OrderLine, _ receivableservice.OrderBillingRequest) (string, error) {
	b.deliveryBatch = append(b.deliveryBatch, batch)
	return "inv-d-" + batch, nil
}

type soWonFake struct{ calls int }

func (w *soWonFake) Evaluate(context.Context, coretenant.Scope, string) (bool, error) {
	w.calls++
	return false, nil
}

type soCounterFake struct{ n int }

func (c *soCounterFake) NextNumber(context.Context, coretenant.Scope, string) (int, error) {
	c.n++
	return c.n, nil
}

type soActivities struct {
	repository.ActivityRepository
	types []domain.ActivityType
}

func (a *soActivities) Create(_ context.Context, _ coretenant.Scope, p repository.CreateActivityParams) (domain.Activity, error) {
	a.types = append(a.types, p.Type)
	return domain.Activity{}, nil
}

type soNotifier struct{ events []string }

func (n *soNotifier) Publish(_ context.Context, e notificationpublisher.Event) (notificationdomain.OutboxEvent, error) {
	n.events = append(n.events, e.Type)
	return notificationdomain.OutboxEvent{}, nil
}

type soMembers struct{ repository.MemberRepository }

func (soMembers) ListActive(context.Context, coretenant.Scope) ([]domain.OrganizationMember, error) {
	return []domain.OrganizationMember{{UserID: "u-owner", Name: "Sari", Email: "sari@example.com"}}, nil
}
func (soMembers) IsActiveMember(context.Context, coretenant.Scope, string) (bool, error) {
	return true, nil
}

type soDeals struct{ repository.DealRepository }

func (soDeals) FindByID(context.Context, coretenant.Scope, string) (domain.Deal, error) {
	return domain.Deal{OwnerUserID: "u-owner"}, nil
}

type soContacts struct{ repository.ContactRepository }

func (soContacts) FindByID(context.Context, coretenant.Scope, string) (domain.Contact, error) {
	return domain.Contact{FirstName: "Budi", LastName: "Santoso", Email: "budi@example.com", Phone: "0812"}, nil
}

type soHarness struct {
	svc        SalesOrderService
	repo       *soRepoFake
	billing    *soBillingFake
	won        *soWonFake
	notifier   *soNotifier
	activities *soActivities
	q1         domain.Quotation
}

func newSOHarness(t *testing.T) *soHarness {
	t.Helper()
	h := &soHarness{repo: &soRepoFake{byID: map[string]domain.SalesOrder{}}, billing: &soBillingFake{}, won: &soWonFake{},
		notifier: &soNotifier{}, activities: &soActivities{}}
	h.svc = NewSalesOrderService(h.repo, h.billing, SalesOrderDeps{
		Counters: &soCounterFake{}, Contacts: soContacts{}, Deals: soDeals{}, Members: soMembers{}, Activities: h.activities,
		Notifier: h.notifier, FrontendURL: "https://app.test",
	}, h.won, func() time.Time { return soNow })
	h.q1 = domain.Quotation{ID: "q1", QuotationNumber: "QUO-2026-0001", DealID: soPtr("d1"), ContactID: soPtr("c1"),
		Items: []domain.QuotationItem{
			{Description: "Instalasi", Quantity: "1", UnitPrice: "500000"},
			{Description: "Internet", Quantity: "1", UnitPrice: "300000", Pricing: pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}},
			{Description: "Website", Quantity: "1", UnitPrice: "5000000", Pricing: pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Postpaid}},
		}}
	return h
}

func (h *soHarness) draft(t *testing.T) domain.SalesOrder {
	t.Helper()
	if err := h.svc.QuotationApproved(soCtx, soScope, h.q1, "u-sales"); err != nil {
		t.Fatal(err)
	}
	so, err := h.repo.FindByQuotation(soCtx, soScope, "q1")
	if err != nil {
		t.Fatal(err)
	}
	return so
}

func (h *soHarness) fill(t *testing.T, id, start string) {
	t.Helper()
	so := h.repo.byID[id]
	if _, err := h.svc.UpdateDraft(soCtx, soScope, id, repository.SalesOrderDraftFields{
		StartDate: soDate(start), BillToName: so.BillToName, BillToEmail: so.BillToEmail, BillToPhone: so.BillToPhone,
		Channels: so.Channels, PICUserID: so.PICUserID}, "u1"); err != nil {
		t.Fatal(err)
	}
}

func TestApprovedHookCreatesDraftOnce(t *testing.T) {
	h := newSOHarness(t)
	_ = h.svc.QuotationApproved(soCtx, soScope, h.q1, "u-sales")
	_ = h.svc.QuotationApproved(soCtx, soScope, h.q1, "u-sales")
	if len(h.repo.byID) != 1 {
		t.Fatalf("orders = %d", len(h.repo.byID))
	}
	so := h.draft(t)
	if so.SONumber != "SO-2026-0001" || so.PICUserID != "u-owner" || so.BillToEmail != "budi@example.com" || so.Channels[0] != "email" || so.BillToName != "Budi Santoso" {
		t.Fatalf("so=%+v", so)
	}
	if len(h.notifier.events) != 1 || h.notifier.events[0] != EventSalesOrderCreated || len(h.activities.types) != 1 || h.activities.types[0] != domain.ActivityTypeOrder {
		t.Fatalf("notify=%v activities=%v", h.notifier.events, h.activities.types)
	}
}

func TestConfirmValidatesAndBills(t *testing.T) {
	h := newSOHarness(t)
	so := h.draft(t)
	_, err := h.svc.Confirm(soCtx, soScope, so.ID, "u1")
	var inc *IncompleteError
	if !errors.As(err, &inc) || len(inc.Fields) != 1 || inc.Fields[0] != "start_date" {
		t.Fatalf("err=%v", err)
	}
	h.fill(t, so.ID, "2026-10-05")
	got, err := h.svc.Confirm(soCtx, soScope, so.ID, "u1")
	if err != nil || got.Status != domain.SalesOrderConfirmed || got.BillingStatus != domain.BillingDone || got.InitialInvoiceID != "inv1" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if h.billing.req.Account.SourceType != "crm_contact" || h.billing.req.StartDate.Format("2006-01-02") != "2026-10-05" || len(h.billing.req.Lines) != 3 {
		t.Fatalf("req=%+v", h.billing.req)
	}
	if h.won.calls != 1 {
		t.Fatal("won evaluated after billing")
	}
	if _, err := h.svc.Confirm(soCtx, soScope, so.ID, "u1"); !errors.Is(err, repository.ErrSalesOrderNotDraft) {
		t.Fatalf("second confirm err=%v", err)
	}
}

func TestConfirmRejectsOldStartDate(t *testing.T) {
	h := newSOHarness(t)
	so := h.draft(t)
	h.fill(t, so.ID, "2026-08-01")
	var inc *IncompleteError
	if _, err := h.svc.Confirm(soCtx, soScope, so.ID, "u1"); !errors.As(err, &inc) {
		t.Fatalf("err=%v", err)
	}
}

func TestConfirmBillingFailureThenRetry(t *testing.T) {
	h := newSOHarness(t)
	so := h.draft(t)
	h.fill(t, so.ID, "2026-10-05")
	h.billing.err = errors.New("boom: asset store down")
	got, err := h.svc.Confirm(soCtx, soScope, so.ID, "u1")
	if err != nil || got.Status != domain.SalesOrderConfirmed || got.BillingStatus != domain.BillingFailed || got.BillingError != billingFailedMessage {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if h.won.calls != 0 {
		t.Fatal("no won evaluation on failure")
	}
	h.billing.err = nil
	got, err = h.svc.RetryBilling(soCtx, soScope, so.ID, "u1")
	if err != nil || got.BillingStatus != domain.BillingDone || got.ContractID != "ctr1" {
		t.Fatalf("retry got=%+v err=%v", got, err)
	}
	if _, err := h.svc.RetryBilling(soCtx, soScope, so.ID, "u1"); !errors.Is(err, ErrBillingNotRetryable) {
		t.Fatalf("retry after done err=%v", err)
	}
}

func TestConfirmDeliveryRequiresPendingItems(t *testing.T) {
	h := newSOHarness(t)
	so := h.draft(t)
	h.fill(t, so.ID, "2026-10-05")
	_, _ = h.svc.Confirm(soCtx, soScope, so.ID, "u1")
	pending, notApplicable := so.Items[2].ID, so.Items[0].ID

	if _, err := h.svc.ConfirmDelivery(soCtx, soScope, so.ID, DeliveryInput{ItemIDs: []string{pending}}, "u1"); !errors.Is(err, ErrBatchKeyRequired) {
		t.Fatalf("no batch err=%v", err)
	}
	if _, err := h.svc.ConfirmDelivery(soCtx, soScope, so.ID, DeliveryInput{ItemIDs: []string{notApplicable}, BatchKey: "b1"}, "u1"); !errors.Is(err, repository.ErrDeliveryNotPending) {
		t.Fatalf("not pending err=%v", err)
	}
	future := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	if _, err := h.svc.ConfirmDelivery(soCtx, soScope, so.ID, DeliveryInput{ItemIDs: []string{pending}, BatchKey: "b1", DeliveredAt: future}, "u1"); !errors.Is(err, ErrDeliveryDateInFuture) {
		t.Fatalf("future err=%v", err)
	}
	callsBefore := h.won.calls
	got, err := h.svc.ConfirmDelivery(soCtx, soScope, so.ID, DeliveryInput{ItemIDs: []string{pending}, BatchKey: "b1"}, "u1")
	if err != nil || got.Items[2].DeliveryStatus != domain.DeliveryDelivered || got.Items[2].InvoiceID != "inv-d-b1" {
		t.Fatalf("got=%+v err=%v", got.Items, err)
	}
	if h.won.calls != callsBefore+1 {
		t.Fatal("won evaluated after delivery")
	}
}

func TestApproveNoLongerSuggestsWon(t *testing.T) {
	if SuggestDealStatusAfterApprove(domain.Quotation{DealID: soPtr("d1")}) != "" {
		t.Fatal("won is decided by the evaluator now")
	}
}
