package service

import (
	"testing"
	"time"

	"zyad.cloud/internal/modules/receivable/domain"
)

func activationInvoiceInput(due *time.Time) InvoiceInput {
	return InvoiceInput{
		AccountID: "a1", SourceType: domain.SourceManual, SourceID: "act-1", IdempotencyKey: "act-1",
		DueDate: due, Lines: []LineInput{line("Halaman undangan budi", "150000")},
	}
}

func TestCreateAndIssueDueDateOverride(t *testing.T) {
	cases := []struct {
		name string
		due  *time.Time
		want string
	}{
		{"default terms (regression)", nil, "2026-10-12"},
		{"+3 days", ptrTime(nowWIB.Add(72 * time.Hour)), "2026-10-08"},
		{"WIB calendar day of the instant", ptrTime(time.Date(2026, 10, 5, 18, 30, 0, 0, time.UTC)), "2026-10-06"},
		{"past is clamped to the issue date", ptrTime(nowWIB.Add(-72 * time.Hour)), "2026-10-05"},
	}
	for _, tc := range cases {
		h := newInvoiceHarness(t)
		in := activationInvoiceInput(tc.due)
		if tc.due == nil {
			in.SourceID, in.IdempotencyKey = "", ""
		}
		inv, err := h.svc.CreateAndIssue(ctx, scope, in, "u1")
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if inv.DueDate == nil || inv.DueDate.Format("2006-01-02") != tc.want {
			t.Fatalf("%s: due = %v, want %s", tc.name, inv.DueDate, tc.want)
		}
	}
}

func TestCreateAndIssueManualWithSourceIsIdempotent(t *testing.T) {
	h := newInvoiceHarness(t)
	due := nowWIB.Add(time.Hour)
	first, err := h.svc.CreateAndIssue(ctx, scope, activationInvoiceInput(&due), "u1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.svc.CreateAndIssue(ctx, scope, activationInvoiceInput(&due), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(h.store.invoices) != 1 {
		t.Fatalf("manual invoice with source must be idempotent: %s vs %s (%d invoices)", first.ID, second.ID, len(h.store.invoices))
	}
	if first.SourceType != domain.SourceManual || first.SourceID != "act-1" {
		t.Fatalf("source = %s/%s", first.SourceType, first.SourceID)
	}
}

func TestCreateAndIssueManualWithoutSourceIsNotDeduplicated(t *testing.T) {
	h := newInvoiceHarness(t)
	in := InvoiceInput{AccountID: "a1", Lines: []LineInput{line("Jasa", "1000")}}
	a, _ := h.svc.CreateAndIssue(ctx, scope, in, "u1")
	b, _ := h.svc.CreateAndIssue(ctx, scope, in, "u1")
	if a.ID == b.ID || len(h.store.invoices) != 2 {
		t.Fatalf("plain manual invoices must not be deduplicated: %s %s", a.ID, b.ID)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
