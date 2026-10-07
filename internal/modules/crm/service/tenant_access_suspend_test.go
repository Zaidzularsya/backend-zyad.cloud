package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
)

var suspendToday = time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)

func day(s string) *time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &d
}

// sBilling meniru pembaca invoice: OverdueDueBefore menyaring dengan due_date < batas seperti repository nyata.
type sBilling struct {
	invoices []BillingInvoice
	linkErr  error
}

func (b *sBilling) OverdueDueBefore(_ context.Context, _ coretenant.Scope, before time.Time) ([]BillingInvoice, error) {
	var out []BillingInvoice
	for _, inv := range b.invoices {
		if inv.Status == "overdue" && inv.DueDate != nil && inv.DueDate.Before(before) {
			out = append(out, inv)
		}
	}
	return out, nil
}

func (b *sBilling) Invoice(_ context.Context, _ coretenant.Scope, id string) (BillingInvoice, error) {
	for _, inv := range b.invoices {
		if inv.ID == id {
			return inv, nil
		}
	}
	return BillingInvoice{}, pgx.ErrNoRows
}

func (b *sBilling) Link(context.Context, coretenant.Scope, string, string) (string, error) {
	return "https://app.test/p/x", b.linkErr
}

type sStatus struct {
	states    map[string]WorkspaceState
	missing   map[string]bool
	suspends  []string
	reactives []string
}

func (s *sStatus) Get(_ context.Context, id string) (WorkspaceState, error) {
	if s.missing[id] {
		return WorkspaceState{}, pgx.ErrNoRows
	}
	return s.states[id], nil
}

func (s *sStatus) SuspendForBilling(_ context.Context, id, number, actor string) error {
	s.suspends = append(s.suspends, id+"/"+number+"/"+actor)
	st := s.states[id]
	st.Status, st.BillingSuspended = "suspended", true
	s.states[id] = st
	return nil
}

func (s *sStatus) ReactivateFromBilling(_ context.Context, id, number, actor string) error {
	s.reactives = append(s.reactives, id+"/"+number+"/"+actor)
	st := s.states[id]
	st.Status, st.BillingSuspended = "active", false
	s.states[id] = st
	return nil
}

type sNotifier struct {
	suspended   []SuspendNotice
	reactivated []string
	err         error
}

func (n *sNotifier) WorkspaceSuspended(_ context.Context, _ string, p SuspendNotice) error {
	n.suspended = append(n.suspended, p)
	return n.err
}

func (n *sNotifier) WorkspaceReactivated(_ context.Context, org, invoice string) error {
	n.reactivated = append(n.reactivated, org+"/"+invoice)
	return nil
}

// sOrders/sCompanies memetakan SO → company → workspace; so-N → co-N → ws-N, company tanpa tautan bila di unlinked.
type sOrders struct {
	repository.SalesOrderRepository
}

func (sOrders) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.SalesOrder, error) {
	co := "co-" + id[len("so-"):]
	return domain.SalesOrder{ID: id, CompanyID: &co}, nil
}

type sCompanies struct {
	repository.CompanyRepository
	unlinked map[string]bool
}

func (c sCompanies) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Company, error) {
	if c.unlinked[id] {
		return domain.Company{}, nil
	}
	ws := "ws-" + id[len("co-"):]
	return domain.Company{TenantOrganizationID: &ws}, nil
}

type suspendFixture struct {
	access   *TenantAccess
	billing  *sBilling
	status   *sStatus
	notifier *sNotifier
}

func newSuspendFixture(invoices []BillingInvoice, states map[string]WorkspaceState) suspendFixture {
	f := suspendFixture{billing: &sBilling{invoices: invoices}, status: &sStatus{states: states, missing: map[string]bool{}}, notifier: &sNotifier{}}
	f.access = NewTenantAccess(sOrders{}, sCompanies{unlinked: map[string]bool{}}, taContracts{}, nil, &taWriter{}).
		WithSuspend(f.billing, f.status, f.notifier, func(context.Context) (string, error) { return "bot", nil })
	return f
}

func overdueInv(n, due string) BillingInvoice {
	return BillingInvoice{ID: "i" + n, Number: "INV-" + n, Status: "overdue", SourceType: string(receivabledomain.SourceSalesOrder),
		SourceID: "so-" + n, DueDate: day(due), AmountDue: "333000.00"}
}

func activeWS(ids ...string) map[string]WorkspaceState {
	m := map[string]WorkspaceState{}
	for _, id := range ids {
		m[id] = WorkspaceState{ID: id, Name: "WS " + id, Status: "active"}
	}
	return m
}

func TestSuspendOverdueGraceBoundary(t *testing.T) {
	// today 2026-10-07, grace 7: due 09-30 (7 hari lalu) belum, due 09-29 (8 hari lalu) disuspend.
	f := newSuspendFixture([]BillingInvoice{overdueInv("1", "2026-09-30"), overdueInv("2", "2026-09-29")}, activeWS("ws-1", "ws-2"))
	res, err := f.access.SuspendOverdue(context.Background(), coretenant.Scope{}, suspendToday, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.status.suspends) != 1 || f.status.suspends[0] != "ws-2/INV-2/bot" || res.Suspended != 1 || res.Checked != 1 {
		t.Fatalf("suspends = %v res = %+v", f.status.suspends, res)
	}
	n := f.notifier.suspended[0]
	if n.InvoiceNumber != "INV-2" || n.InvoiceURL != "https://app.test/p/x" || n.GraceDays != 7 || n.WorkspaceName != "WS ws-2" || n.AmountDue != "Rp 333.000" {
		t.Fatalf("notice = %+v", n)
	}
}

func TestSuspendOverdueRunTwiceSuspendsAndNotifiesOnce(t *testing.T) {
	f := newSuspendFixture([]BillingInvoice{overdueInv("1", "2026-09-01")}, activeWS("ws-1"))
	for i := 0; i < 2; i++ {
		if _, err := f.access.SuspendOverdue(context.Background(), coretenant.Scope{}, suspendToday, 7); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.status.suspends) != 1 || len(f.notifier.suspended) != 1 {
		t.Fatalf("suspends = %d notices = %d, want 1/1", len(f.status.suspends), len(f.notifier.suspended))
	}
}

func TestSuspendOverdueSkipsManuallySuspended(t *testing.T) {
	states := activeWS("ws-1")
	states["ws-1"] = WorkspaceState{ID: "ws-1", Status: "suspended"} // tanpa penanda billing
	f := newSuspendFixture([]BillingInvoice{overdueInv("1", "2026-09-01")}, states)
	res, err := f.access.SuspendOverdue(context.Background(), coretenant.Scope{}, suspendToday, 7)
	if err != nil || len(f.status.suspends) != 0 || len(f.notifier.suspended) != 0 || res.Skipped != 1 || res.Suspended != 0 {
		t.Fatalf("suspends=%v res=%+v err=%v", f.status.suspends, res, err)
	}
	if f.status.states["ws-1"].BillingSuspended {
		t.Fatal("manual suspension must not get the billing marker")
	}
}

func TestSuspendOverdueSuspendsAgainAfterManualActivation(t *testing.T) {
	f := newSuspendFixture([]BillingInvoice{overdueInv("1", "2026-09-01")}, activeWS("ws-1"))
	ctx := context.Background()
	if _, err := f.access.SuspendOverdue(ctx, coretenant.Scope{}, suspendToday, 7); err != nil {
		t.Fatal(err)
	}
	f.status.states["ws-1"] = WorkspaceState{ID: "ws-1", Name: "WS ws-1", Status: "active"} // admin mengaktifkan manual
	if _, err := f.access.SuspendOverdue(ctx, coretenant.Scope{}, suspendToday, 7); err != nil {
		t.Fatal(err)
	}
	if len(f.status.suspends) != 2 {
		t.Fatalf("suspends = %v, want 2", f.status.suspends)
	}
}

func TestSuspendOverdueMissingWorkspaceDoesNotStopOthers(t *testing.T) {
	f := newSuspendFixture([]BillingInvoice{overdueInv("1", "2026-09-01"), overdueInv("2", "2026-09-02")}, activeWS("ws-2"))
	f.status.missing["ws-1"] = true
	res, err := f.access.SuspendOverdue(context.Background(), coretenant.Scope{}, suspendToday, 7)
	if err != nil {
		t.Fatalf("missing workspace must not be an error: %v", err)
	}
	if len(f.status.suspends) != 1 || f.status.suspends[0] != "ws-2/INV-2/bot" || res.Skipped != 1 {
		t.Fatalf("suspends = %v res = %+v", f.status.suspends, res)
	}
}

func TestSuspendOverdueSkipsUnlinkedCompany(t *testing.T) {
	f := newSuspendFixture([]BillingInvoice{overdueInv("1", "2026-09-01")}, activeWS("ws-1"))
	f.access.companies = sCompanies{unlinked: map[string]bool{"co-1": true}}
	res, err := f.access.SuspendOverdue(context.Background(), coretenant.Scope{}, suspendToday, 7)
	if err != nil || len(f.status.suspends) != 0 || res.Skipped != 1 {
		t.Fatalf("suspends=%v res=%+v err=%v", f.status.suspends, res, err)
	}
}

func TestSuspendOverdueUsesOldestInvoiceAndCountsNotifyFailure(t *testing.T) {
	inv1, inv2 := overdueInv("1", "2026-09-10"), overdueInv("1", "2026-09-01")
	inv2.ID, inv2.Number = "i9", "INV-OLD"
	f := newSuspendFixture([]BillingInvoice{inv1, inv2}, activeWS("ws-1"))
	f.notifier.err = errors.New("smtp down")
	res, err := f.access.SuspendOverdue(context.Background(), coretenant.Scope{}, suspendToday, 7)
	if err != nil {
		t.Fatalf("notification failure must not be an error: %v", err)
	}
	if res.Suspended != 1 || f.status.suspends[0] != "ws-1/INV-OLD/bot" {
		t.Fatalf("res=%+v suspends=%v", res, f.status.suspends)
	}
}

// ---- Task 4: aktif kembali ----

func billingSuspended(ids ...string) map[string]WorkspaceState {
	m := map[string]WorkspaceState{}
	for _, id := range ids {
		m[id] = WorkspaceState{ID: id, Name: "WS " + id, Status: "suspended", BillingSuspended: true}
	}
	return m
}

func paid(inv BillingInvoice) BillingInvoice {
	inv.Status = "paid"
	return inv
}

func TestReactivateWhenOnlyInvoiceSettled(t *testing.T) {
	f := newSuspendFixture([]BillingInvoice{paid(overdueInv("1", "2026-09-01"))}, billingSuspended("ws-1"))
	if err := f.access.ReactivateIfSettled(context.Background(), coretenant.Scope{}, "i1", suspendToday, 7); err != nil {
		t.Fatal(err)
	}
	if len(f.status.reactives) != 1 || f.status.reactives[0] != "ws-1/INV-1/bot" || len(f.notifier.reactivated) != 1 || f.notifier.reactivated[0] != "ws-1/INV-1" {
		t.Fatalf("reactives = %v notices = %v", f.status.reactives, f.notifier.reactivated)
	}
}

func TestReactivateWaitsForAllOverdueInvoices(t *testing.T) {
	a, b := overdueInv("1", "2026-09-01"), overdueInv("2", "2026-09-02")
	b.SourceID = "so-1" // dua invoice, workspace yang sama (ws-1)
	f := newSuspendFixture([]BillingInvoice{paid(a), b}, billingSuspended("ws-1"))
	ctx := context.Background()
	if err := f.access.ReactivateIfSettled(ctx, coretenant.Scope{}, "i1", suspendToday, 7); err != nil {
		t.Fatal(err)
	}
	if len(f.status.reactives) != 0 {
		t.Fatalf("reactivated while another invoice is still overdue: %v", f.status.reactives)
	}
	f.billing.invoices[1] = paid(b)
	if err := f.access.ReactivateIfSettled(ctx, coretenant.Scope{}, "i2", suspendToday, 7); err != nil {
		t.Fatal(err)
	}
	if len(f.status.reactives) != 1 || f.status.reactives[0] != "ws-1/INV-2/bot" {
		t.Fatalf("reactives = %v", f.status.reactives)
	}
}

func TestReactivateLeavesManualSuspensionAlone(t *testing.T) {
	states := map[string]WorkspaceState{"ws-1": {ID: "ws-1", Status: "suspended"}} // tanpa penanda
	f := newSuspendFixture([]BillingInvoice{paid(overdueInv("1", "2026-09-01"))}, states)
	if err := f.access.ReactivateIfSettled(context.Background(), coretenant.Scope{}, "i1", suspendToday, 7); err != nil {
		t.Fatal(err)
	}
	if len(f.status.reactives) != 0 || len(f.notifier.reactivated) != 0 {
		t.Fatalf("manual suspension reactivated: %v", f.status.reactives)
	}
}

func TestReactivateNoopForActiveOrUnlinked(t *testing.T) {
	f := newSuspendFixture([]BillingInvoice{paid(overdueInv("1", "2026-09-01"))}, activeWS("ws-1"))
	if err := f.access.ReactivateIfSettled(context.Background(), coretenant.Scope{}, "i1", suspendToday, 7); err != nil || len(f.status.reactives) != 0 {
		t.Fatalf("active workspace: reactives=%v err=%v", f.status.reactives, err)
	}
	f = newSuspendFixture([]BillingInvoice{paid(overdueInv("1", "2026-09-01"))}, billingSuspended("ws-1"))
	f.access.companies = sCompanies{unlinked: map[string]bool{"co-1": true}}
	if err := f.access.ReactivateIfSettled(context.Background(), coretenant.Scope{}, "i1", suspendToday, 7); err != nil || len(f.status.reactives) != 0 {
		t.Fatalf("unlinked: reactives=%v err=%v", f.status.reactives, err)
	}
}

type orderRecorder struct{ calls *[]string }

func (o orderRecorder) SyncContract(context.Context, coretenant.Scope, string, SyncReason) error {
	*o.calls = append(*o.calls, "sync")
	return nil
}

func (o orderRecorder) ReactivateIfSettled(context.Context, coretenant.Scope, string, time.Time, int) error {
	*o.calls = append(*o.calls, "reactivate")
	return nil
}

func TestListenerReactivatesAfterSyncContract(t *testing.T) {
	var calls []string
	rec := orderRecorder{calls: &calls}
	l := NewReceivableListener(nil, nil, nil).WithTenantAccess(rec).WithReactivation(rec, 7)
	l.now = func() time.Time { return suspendToday }
	l.InvoicePaid(context.Background(), coretenant.Scope{}, receivableservice.InvoiceRef{ID: "i1", ContractID: "ctr-1"})
	if len(calls) != 2 || calls[0] != "sync" || calls[1] != "reactivate" {
		t.Fatalf("calls = %v, want sync then reactivate", calls)
	}
}
