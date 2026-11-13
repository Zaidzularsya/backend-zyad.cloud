package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/shared/pricing"
)

// ---- fake arrears repository over the invoice harness store ----

type fakeArrears struct {
	store      *fakeStore
	recordings map[string]domain.ArrearsRecording
	extra      []repository.PeriodRange // periods billed by invoices outside the store (initial SO invoice)
	flags      int                      // SetIsolirFlag changes
}

func newFakeArrears(store *fakeStore) *fakeArrears {
	return &fakeArrears{store: store, recordings: map[string]domain.ArrearsRecording{}}
}

func (f *fakeArrears) CreateRecording(_ context.Context, _ coretenant.Scope, p repository.CreateRecordingParams) (domain.ArrearsRecording, error) {
	for _, r := range f.recordings {
		if r.IdempotencyKey == p.IdempotencyKey {
			return domain.ArrearsRecording{}, repository.ErrDuplicateRecording
		}
	}
	r := domain.ArrearsRecording{ID: fmt.Sprintf("rec-%d", len(f.recordings)+1), AccountID: p.AccountID, ContractID: p.ContractID,
		Reason: p.Reason, RecordedBy: p.RecordedBy, IdempotencyKey: p.IdempotencyKey, NotifyStatus: domain.ArrearsNotifyPending}
	f.recordings[r.ID] = r
	return r, nil
}
func (f *fakeArrears) FindRecordingByKey(_ context.Context, _ coretenant.Scope, key string) (domain.ArrearsRecording, bool, error) {
	for _, r := range f.recordings {
		if r.IdempotencyKey == key {
			return r, true, nil
		}
	}
	return domain.ArrearsRecording{}, false, nil
}
func (f *fakeArrears) GetRecording(_ context.Context, _ coretenant.Scope, id string) (domain.ArrearsRecording, error) {
	r, ok := f.recordings[id]
	if !ok {
		return r, pgx.ErrNoRows
	}
	return r, nil
}
func (f *fakeArrears) SetNotify(_ context.Context, _ coretenant.Scope, id, status, errText string) error {
	r := f.recordings[id]
	r.NotifyStatus, r.NotifyError = status, errText
	f.recordings[id] = r
	return nil
}
func (f *fakeArrears) ReleaseRecordingKey(_ context.Context, _ coretenant.Scope, id string) error {
	r := f.recordings[id]
	r.IdempotencyKey = "released:" + id
	f.recordings[id] = r
	return nil
}
func (f *fakeArrears) InvoicesByRecording(_ context.Context, _ coretenant.Scope, id string) ([]domain.Invoice, error) {
	out := []domain.Invoice{}
	for i := 1; i <= f.store.seq; i++ {
		if inv, ok := f.store.invoices[fmt.Sprintf("inv-%d", i)]; ok && inv.ArrearsRecordingID == id {
			out = append(out, inv)
		}
	}
	return out, nil
}
func (f *fakeArrears) ExistingPeriods(_ context.Context, _ coretenant.Scope, contractID string) ([]repository.PeriodRange, error) {
	out := append([]repository.PeriodRange{}, f.extra...)
	for _, inv := range f.store.invoices {
		if inv.ContractID == contractID && inv.Status != domain.InvoiceVoid && inv.PeriodStart != nil && inv.PeriodEnd != nil {
			out = append(out, repository.PeriodRange{Start: *inv.PeriodStart, End: *inv.PeriodEnd})
		}
	}
	return out, nil
}
func (f *fakeArrears) SetIsolirFlag(_ context.Context, _ coretenant.Scope, id string, value bool, _ string) (bool, error) {
	inv, ok := f.store.invoices[id]
	if !ok {
		return false, pgx.ErrNoRows
	}
	if inv.Status != domain.InvoiceIssued && inv.Status != domain.InvoiceOverdue {
		return false, repository.ErrInvoiceNotOpen
	}
	if inv.IsolirFlag == value {
		return false, nil
	}
	inv.IsolirFlag = value
	f.store.invoices[id] = inv
	f.flags++
	return true, nil
}

type fakeTriggerProducts struct {
	enabled map[string]bool
	asked   []string
}

func (f *fakeTriggerProducts) AnyTriggerProduct(_ context.Context, _ coretenant.Scope, productIDs []string) (bool, error) {
	f.asked = productIDs
	for _, id := range productIDs {
		if f.enabled[id] {
			return true, nil
		}
	}
	return false, nil
}

type fakeArrearsNotifier struct {
	calls    int
	invoices []domain.Invoice
	status   string
}

func (f *fakeArrearsNotifier) NotifyArrears(_ context.Context, _ coretenant.Scope, _ domain.ArrearsRecording, invs []domain.Invoice, _ string) (string, string) {
	f.calls++
	f.invoices = invs
	if f.status == "" {
		return domain.ArrearsNotifySent, ""
	}
	return f.status, "gagal"
}

type arrearsHarness struct {
	*invoiceHarness
	svc       ArrearsService
	repo      *fakeArrears
	contracts *fakeContracts
	contract  domain.Contract
	triggers  *fakeTriggerProducts
	notifier  *fakeArrearsNotifier
}

// newArrearsHarness: today (WIB) = 2026-10-05; contract monthly from 2026-07-05, 300.000 + 11% tax.
func newArrearsHarness(t *testing.T) *arrearsHarness {
	t.Helper()
	h := &arrearsHarness{invoiceHarness: newInvoiceHarness(t), contracts: newFakeContracts(),
		triggers: &fakeTriggerProducts{enabled: map[string]bool{"prod-1": true}}, notifier: &fakeArrearsNotifier{}}
	h.repo = newFakeArrears(h.store)
	c, err := h.contracts.Create(ctx, scope, repository.CreateContractParams{
		Number: "CTR-2026-0001", AccountID: "a1", SourceType: "sales_order", SourceID: "so-1", StartDate: *day("2026-07-05"),
		Channels: []string{"email"}, Items: []repository.ContractItemParams{{
			Description: "Internet", Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.Items[0].Quantity, c.Items[0].UnitPrice, c.Items[0].TaxPercent, c.Items[0].ProductID = "1", "300000", "11", "prod-1"
	h.contracts.byID[c.ID] = c
	h.contract = c
	h.svc = NewArrearsService(ArrearsDeps{
		Arrears: h.repo, Contracts: h.contracts, Invoices: h.invoiceHarness.svc, Triggers: h.triggers, Notifier: h.notifier,
		Now: func() time.Time { return nowWIB },
	})
	return h
}

func (h *arrearsHarness) input(key string, rows ...ArrearsRowInput) ArrearsInput {
	return ArrearsInput{AccountID: "a1", ContractID: h.contract.ID, Reason: "Tunggakan sebelum pakai sistem", IdempotencyKey: key, Rows: rows}
}

func row(start, amount, due string, flag bool) ArrearsRowInput {
	return ArrearsRowInput{PeriodStart: *day(start), Amount: amount, DueDate: *day(due), IsolirFlag: flag}
}

func TestArrearsRecord_ThreeRowsBackdatedWithoutClampOrAutoSend(t *testing.T) {
	h := newArrearsHarness(t)
	res, err := h.svc.Record(ctx, scope, h.input("k1",
		row("2026-07-05", "333000", "2026-07-12", false),
		row("2026-08-05", "333000", "2026-08-12", true),
		row("2026-09-05", "250000", "2026-09-12", false),
	), "u1")
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(res.Invoices) != 3 || h.sender.autoCalls != 0 {
		t.Fatalf("invoices=%d autoSend=%d", len(res.Invoices), h.sender.autoCalls)
	}
	wantDue := []string{"2026-07-12", "2026-08-12", "2026-09-12"}
	for i, inv := range res.Invoices {
		if inv.Status != domain.InvoiceOverdue || inv.DueDate == nil || inv.DueDate.Format("2006-01-02") != wantDue[i] {
			t.Fatalf("invoice %d status=%s due=%v", i, inv.Status, inv.DueDate)
		}
		if inv.ContractID != h.contract.ID || inv.ArrearsRecordingID != res.Recording.ID || inv.IsolirFlag != (i == 1) {
			t.Fatalf("invoice %d = %+v", i, inv)
		}
		if inv.InvoiceNumber == "" || inv.PeriodStart == nil || inv.PeriodEnd == nil || len(inv.Items) != 1 ||
			inv.Items[0].ContractItemID != "" || !strings.HasPrefix(inv.Items[0].Description, "Tunggakan ") {
			t.Fatalf("invoice %d shape = %+v", i, inv)
		}
	}
	if res.Invoices[0].PeriodEnd.Format("2006-01-02") != "2026-08-04" || res.Invoices[2].GrandTotal != "250000.00" {
		t.Fatalf("period/amount = %v %s", res.Invoices[0].PeriodEnd, res.Invoices[2].GrandTotal)
	}
	if res.Recording.RecordedBy != "u1" || res.Recording.Reason != "Tunggakan sebelum pakai sistem" {
		t.Fatalf("recording = %+v", res.Recording)
	}
	if h.notifier.calls != 1 || len(h.notifier.invoices) != 3 || res.Recording.NotifyStatus != domain.ArrearsNotifySent {
		t.Fatalf("notify calls=%d invoices=%d status=%s", h.notifier.calls, len(h.notifier.invoices), res.Recording.NotifyStatus)
	}
}

func TestArrearsRecord_DueTodayStaysIssued(t *testing.T) {
	h := newArrearsHarness(t)
	res, err := h.svc.Record(ctx, scope, h.input("k1", row("2026-09-05", "333000", "2026-10-05", false)), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Invoices[0].Status != domain.InvoiceIssued || res.Invoices[0].DueDate.Format("2006-01-02") != "2026-10-05" {
		t.Fatalf("invoice = %+v", res.Invoices[0])
	}
}

func TestArrearsRecord_SameKeyReturnsFirstResult(t *testing.T) {
	h := newArrearsHarness(t)
	in := h.input("k1", row("2026-07-05", "333000", "2026-07-12", false), row("2026-08-05", "333000", "2026-08-12", false))
	first, err := h.svc.Record(ctx, scope, in, "u1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.svc.Record(ctx, scope, in, "u1")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if second.Recording.ID != first.Recording.ID || len(second.Invoices) != 2 || len(h.store.invoices) != 2 || h.notifier.calls != 1 {
		t.Fatalf("replay recording=%s invoices=%d stored=%d notify=%d", second.Recording.ID, len(second.Invoices), len(h.store.invoices), h.notifier.calls)
	}
}

func conflictsOf(t *testing.T, err error) []ArrearsConflict {
	t.Helper()
	var ce *ArrearsConflictError
	if !errors.Is(err, ErrArrearsConflict) || !errors.As(err, &ce) {
		t.Fatalf("err = %v, want ErrArrearsConflict", err)
	}
	return ce.Conflicts
}

func TestArrearsRecord_ConflictWithExistingInvoiceCreatesNothing(t *testing.T) {
	h := newArrearsHarness(t)
	// The initial invoice billed the current cycle 2026-09-05..2026-10-04.
	h.repo.extra = []repository.PeriodRange{{Start: *day("2026-09-05"), End: *day("2026-10-04")}}
	_, err := h.svc.Record(ctx, scope, h.input("k1",
		row("2026-08-05", "333000", "2026-08-12", false), row("2026-09-05", "333000", "2026-09-12", false)), "u1")
	c := conflictsOf(t, err)
	if len(c) != 1 || c[0].Row != 2 || c[0].PeriodStart != "2026-09-05" || !strings.Contains(c[0].Reason, "sudah") {
		t.Fatalf("conflicts = %+v", c)
	}
	if len(h.store.invoices) != 0 || len(h.repo.recordings) != 0 {
		t.Fatalf("invoices=%d recordings=%d, want none", len(h.store.invoices), len(h.repo.recordings))
	}
}

func TestArrearsRecord_DuplicateRowsRejected(t *testing.T) {
	h := newArrearsHarness(t)
	_, err := h.svc.Record(ctx, scope, h.input("k1",
		row("2026-08-05", "333000", "2026-08-12", false), row("2026-08-05", "100000", "2026-08-20", false)), "u1")
	c := conflictsOf(t, err)
	if len(c) != 1 || c[0].Row != 2 {
		t.Fatalf("conflicts = %+v", c)
	}
	if len(h.store.invoices) != 0 {
		t.Fatalf("invoices created: %d", len(h.store.invoices))
	}
}

func TestArrearsRecord_VoidedPeriodCanBeRecordedAgain(t *testing.T) {
	h := newArrearsHarness(t)
	first, err := h.svc.Record(ctx, scope, h.input("k1", row("2026-08-05", "333000", "2026-08-12", false)), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.invoiceHarness.svc.Void(ctx, scope, first.Invoices[0].ID, "salah catat", "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Record(ctx, scope, h.input("k2", row("2026-08-05", "300000", "2026-08-12", false)), "u1"); err != nil {
		t.Fatalf("re-record voided period: %v", err)
	}
	if _, err := h.svc.Record(ctx, scope, h.input("k3", row("2026-08-05", "300000", "2026-08-12", false)), "u1"); !errors.Is(err, ErrArrearsConflict) {
		t.Fatalf("third record err = %v, want conflict", err)
	}
}

func TestArrearsRecord_InvalidInput(t *testing.T) {
	many := []ArrearsRowInput{}
	for i := 0; i < 25; i++ {
		many = append(many, row("2026-08-05", "1", "2026-08-12", false))
	}
	cases := map[string]func(h *arrearsHarness) ArrearsInput{
		"empty reason": func(h *arrearsHarness) ArrearsInput {
			in := h.input("k1", row("2026-08-05", "333000", "2026-08-12", false))
			in.Reason = "  "
			return in
		},
		"reason too long": func(h *arrearsHarness) ArrearsInput {
			in := h.input("k1", row("2026-08-05", "333000", "2026-08-12", false))
			in.Reason = strings.Repeat("a", 501)
			return in
		},
		"no rows":           func(h *arrearsHarness) ArrearsInput { return h.input("k1") },
		"more than 24 rows": func(h *arrearsHarness) ArrearsInput { return h.input("k1", many...) },
		"future due date": func(h *arrearsHarness) ArrearsInput {
			return h.input("k1", row("2026-08-05", "333000", "2026-10-06", false))
		},
		"not a cycle start": func(h *arrearsHarness) ArrearsInput {
			return h.input("k1", row("2026-08-06", "333000", "2026-08-12", false))
		},
		"before contract start": func(h *arrearsHarness) ArrearsInput {
			return h.input("k1", row("2026-06-05", "333000", "2026-06-12", false))
		},
		"period starting after today": func(h *arrearsHarness) ArrearsInput {
			return h.input("k1", row("2026-11-05", "333000", "2026-10-01", false))
		},
		"zero amount": func(h *arrearsHarness) ArrearsInput {
			return h.input("k1", row("2026-08-05", "0", "2026-08-12", false))
		},
		"bad amount": func(h *arrearsHarness) ArrearsInput {
			return h.input("k1", row("2026-08-05", "12.345", "2026-08-12", false))
		},
		"missing key": func(h *arrearsHarness) ArrearsInput {
			return h.input("", row("2026-08-05", "333000", "2026-08-12", false))
		},
	}
	for name, build := range cases {
		h := newArrearsHarness(t)
		_, err := h.svc.Record(ctx, scope, build(h), "u1")
		if !errors.Is(err, ErrArrearsInvalid) {
			t.Fatalf("%s: err = %v, want ErrArrearsInvalid", name, err)
		}
		if len(h.store.invoices) != 0 || len(h.repo.recordings) != 0 {
			t.Fatalf("%s: wrote invoices=%d recordings=%d", name, len(h.store.invoices), len(h.repo.recordings))
		}
	}
}

func TestArrearsRecord_ContractNotOfAccount(t *testing.T) {
	h := newArrearsHarness(t)
	in := h.input("k1", row("2026-08-05", "333000", "2026-08-12", false))
	in.AccountID = "someone-else"
	if _, err := h.svc.Record(ctx, scope, in, "u1"); !errors.Is(err, ErrArrearsContractMissing) {
		t.Fatalf("err = %v, want ErrArrearsContractMissing", err)
	}
	in = h.input("k1", row("2026-08-05", "333000", "2026-08-12", false))
	in.ContractID = "ctr-missing"
	if _, err := h.svc.Record(ctx, scope, in, "u1"); !errors.Is(err, ErrArrearsContractMissing) {
		t.Fatalf("err = %v, want ErrArrearsContractMissing", err)
	}
}

func TestArrearsRecord_FailureMidwayVoidsCreatedInvoicesAndFreesKey(t *testing.T) {
	h := newArrearsHarness(t)
	h.store.failCreateAfter = 1 // the second invoice fails to be created
	_, err := h.svc.Record(ctx, scope, h.input("k1",
		row("2026-07-05", "333000", "2026-07-12", false), row("2026-08-05", "333000", "2026-08-12", false)), "u1")
	if err == nil {
		t.Fatal("want error")
	}
	for _, inv := range h.store.invoices {
		if inv.Status != domain.InvoiceVoid {
			t.Fatalf("invoice %s left %s, want void", inv.ID, inv.Status)
		}
	}
	h.store.failCreateAfter = 0
	res, err := h.svc.Record(ctx, scope, h.input("k1",
		row("2026-07-05", "333000", "2026-07-12", false), row("2026-08-05", "333000", "2026-08-12", false)), "u1")
	if err != nil || len(res.Invoices) != 2 {
		t.Fatalf("retry with same key: invoices=%d err=%v", len(res.Invoices), err)
	}
}

func TestArrearsRecord_NotifyFailureDoesNotFail(t *testing.T) {
	h := newArrearsHarness(t)
	h.notifier.status = domain.ArrearsNotifyFailed
	res, err := h.svc.Record(ctx, scope, h.input("k1", row("2026-08-05", "333000", "2026-08-12", false)), "u1")
	if err != nil || len(res.Invoices) != 1 || res.Recording.NotifyStatus != domain.ArrearsNotifyFailed {
		t.Fatalf("res=%+v err=%v", res.Recording, err)
	}
	h.notifier.status = ""
	rec, err := h.svc.Resend(ctx, scope, res.Recording.ID, "u1")
	if err != nil || rec.NotifyStatus != domain.ArrearsNotifySent || h.notifier.calls != 2 {
		t.Fatalf("resend = %+v err=%v calls=%d", rec, err, h.notifier.calls)
	}
}

func TestArrearsPreview_DefaultsConflictsAndTriggerProduct(t *testing.T) {
	h := newArrearsHarness(t)
	h.repo.extra = []repository.PeriodRange{{Start: *day("2026-09-05"), End: *day("2026-10-04")}}
	rows, hasTrigger, err := h.svc.Preview(ctx, scope, h.contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTrigger || len(h.triggers.asked) != 1 || h.triggers.asked[0] != "prod-1" {
		t.Fatalf("hasTrigger=%v asked=%v", hasTrigger, h.triggers.asked)
	}
	if len(rows) != 4 || rows[0].Period.DefaultAmount != "333000.00" {
		t.Fatalf("rows = %+v", rows)
	}
	for i, r := range rows {
		if (r.Conflict != "") != (i == 2) {
			t.Fatalf("row %d conflict = %q", i, r.Conflict)
		}
	}
	if _, _, err := h.svc.Preview(ctx, scope, "ctr-missing"); !errors.Is(err, ErrArrearsContractMissing) {
		t.Fatalf("missing contract err = %v", err)
	}
	h.triggers.enabled = map[string]bool{}
	if _, hasTrigger, _ := h.svc.Preview(ctx, scope, h.contract.ID); hasTrigger {
		t.Fatal("no enabled trigger product → false")
	}
}

func TestArrearsSetIsolirFlag(t *testing.T) {
	h := newArrearsHarness(t)
	res, err := h.svc.Record(ctx, scope, h.input("k1", row("2026-08-05", "333000", "2026-08-12", false)), "u1")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := h.svc.SetIsolirFlag(ctx, scope, res.Invoices[0].ID, true, "owner")
	if err != nil || !inv.IsolirFlag || h.repo.flags != 1 {
		t.Fatalf("set flag inv=%v err=%v flags=%d", inv.IsolirFlag, err, h.repo.flags)
	}
	normal := h.draft(t, InvoiceInput{})
	if _, err := h.svc.SetIsolirFlag(ctx, scope, normal.ID, true, "owner"); !errors.Is(err, ErrArrearsInvalid) {
		t.Fatalf("non-arrears invoice err = %v", err)
	}
	if _, err := h.invoiceHarness.svc.Void(ctx, scope, res.Invoices[0].ID, "salah", "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SetIsolirFlag(ctx, scope, res.Invoices[0].ID, false, "owner"); !errors.Is(err, ErrArrearsInvalid) {
		t.Fatalf("void invoice err = %v", err)
	}
}

// C1: a back-dated due date must not produce a pay link that is already expired.
func TestLinkExpiry_BackdatedDueUsesIssueDate(t *testing.T) {
	inv := domain.Invoice{IssueDate: day("2026-11-13"), DueDate: day("2026-06-15")}
	got := LinkExpiry(inv, nowWIB)
	want := time.Date(2027, 2, 11, 23, 59, 59, 0, businesstime.Location()).UTC()
	if !got.Equal(want) {
		t.Fatalf("expiry = %v, want %v", got, want)
	}
	// Normal invoices (due >= issue) keep the due-date rule.
	normal := domain.Invoice{IssueDate: day("2026-10-05"), DueDate: day("2026-10-12")}
	if want := time.Date(2027, 1, 10, 23, 59, 59, 0, businesstime.Location()).UTC(); !LinkExpiry(normal, nowWIB).Equal(want) {
		t.Fatalf("normal expiry = %v", LinkExpiry(normal, nowWIB))
	}
}

func TestArrearsRecord_OldDueGetsLiveLink(t *testing.T) {
	h := newArrearsHarness(t)
	if _, err := h.svc.Record(ctx, scope, h.input("k1", row("2026-07-05", "333000", "2026-07-05", false)), "u1"); err != nil {
		t.Fatal(err)
	}
	if !h.links.lastExpiry.After(nowWIB.AddDate(0, 0, 89)) {
		t.Fatalf("link expires %v, before now+89d", h.links.lastExpiry)
	}
}

func TestArrearsRecord_PeriodTheBillingRunWillBillIsRejected(t *testing.T) {
	h := newArrearsHarness(t)
	c := h.contracts.byID[h.contract.ID]
	c.Items[0].NextPeriodStart = *day("2026-09-05")
	h.contracts.byID[c.ID] = c
	_, err := h.svc.Record(ctx, scope, h.input("k1", row("2026-09-05", "333000", "2026-09-12", false)), "u1")
	if !errors.Is(err, ErrArrearsInvalid) || !strings.Contains(err.Error(), "ditagih otomatis") {
		t.Fatalf("err = %v", err)
	}
	if _, err := h.svc.Record(ctx, scope, h.input("k2", row("2026-08-05", "333000", "2026-08-12", false)), "u1"); err != nil {
		t.Fatalf("period before the pointer: %v", err)
	}
}

// I2: a draft whose issue failed is voided too, so the same dialog can retry.
func TestArrearsRecord_IssueFailureVoidsTheLeftoverDraft(t *testing.T) {
	h := newArrearsHarness(t)
	h.store.failIssueAfter = 1 // the second row's draft is created, its issue fails
	in := h.input("k1", row("2026-07-05", "333000", "2026-07-12", false), row("2026-08-05", "333000", "2026-08-12", false))
	if _, err := h.svc.Record(ctx, scope, in, "u1"); err == nil {
		t.Fatal("want error")
	}
	if len(h.store.invoices) != 2 {
		t.Fatalf("invoices = %d, want 2 (one issued, one draft)", len(h.store.invoices))
	}
	for _, inv := range h.store.invoices {
		if inv.Status != domain.InvoiceVoid {
			t.Fatalf("invoice %s left %s, want void", inv.ID, inv.Status)
		}
	}
	h.store.failIssueAfter = 0
	if res, err := h.svc.Record(ctx, scope, in, "u1"); err != nil || len(res.Invoices) != 2 {
		t.Fatalf("retry: invoices=%d err=%v", len(res.Invoices), err)
	}
}
