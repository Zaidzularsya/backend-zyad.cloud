package service

import (
	"time"

	"errors"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/shared/pricing"
)

func TestIssueAssignsNumberDatesAndAutoSends(t *testing.T) {
	h := newInvoiceHarness(t) // now = 2026-10-05 10:00 WIB, settings terms 7
	inv := h.draft(t, InvoiceInput{})
	got, err := h.svc.Issue(ctx, scope, inv.ID, "u1")
	if err != nil || got.InvoiceNumber != "INV-2026-0001" || got.IssueDate.Format("2006-01-02") != "2026-10-05" || got.DueDate.Format("2006-01-02") != "2026-10-12" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if h.sender.autoCalls != 1 || h.files.uploads != 1 || h.links.ensured != 1 {
		t.Fatalf("send=%d pdf=%d link=%d", h.sender.autoCalls, h.files.uploads, h.links.ensured)
	}
	if h.sender.last.PDFAssetID == nil {
		t.Fatal("auto send must receive the invoice with its PDF snapshot id")
	}
	if want := "2027-01-10"; h.links.lastExpiry.In(wib()).Format("2006-01-02") != want {
		t.Fatalf("link expiry = %v, want %s (due date + 90 days)", h.links.lastExpiry, want)
	}
	again, _ := h.svc.Issue(ctx, scope, inv.ID, "u1")
	if again.InvoiceNumber != "INV-2026-0001" || h.sender.autoCalls != 1 || h.counter.calls != 1 {
		t.Fatal("second issue must be a no-op")
	}
}

func TestIssueUsesBusinessDayAroundMidnight(t *testing.T) {
	h := newInvoiceHarness(t)
	// 2026-10-05 20:00 UTC = 2026-10-06 03:00 WIB → tanggal terbit 6 Okt.
	h.svc.now = func() time.Time { return time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC) }
	inv := h.draft(t, InvoiceInput{})
	got, _ := h.svc.Issue(ctx, scope, inv.ID, "u1")
	if got.IssueDate.Format("2006-01-02") != "2026-10-06" || got.InvoiceNumber != "INV-2026-0001" {
		t.Fatalf("issue date = %v number = %s", got.IssueDate, got.InvoiceNumber)
	}
}

func TestPrepaidPeriodDueDateNotBeforePeriodStart(t *testing.T) {
	h := newInvoiceHarness(t)
	inv := h.draft(t, InvoiceInput{PeriodStart: day("2026-10-20"), PeriodEnd: day("2026-11-19")})
	got, err := h.svc.Issue(ctx, scope, inv.ID, "u1")
	if err != nil || got.DueDate.Format("2006-01-02") != "2026-10-20" {
		t.Fatalf("due = %v err=%v (issue 05 Okt + 7 hari < period_start → 20 Okt)", got.DueDate, err)
	}
	// Periode sudah lewat (pascabayar): termin biasa.
	post := h.draft(t, InvoiceInput{PeriodStart: day("2026-09-01"), PeriodEnd: day("2026-09-30")})
	got, _ = h.svc.Issue(ctx, scope, post.ID, "u1")
	if got.DueDate.Format("2006-01-02") != "2026-10-12" {
		t.Fatalf("postpaid due = %v", got.DueDate)
	}
}

func TestIssueRaceLoserReturnsExistingWithoutSending(t *testing.T) {
	h := newInvoiceHarness(t)
	inv := h.draft(t, InvoiceInput{})
	h.store.issueRace = true
	got, err := h.svc.Issue(ctx, scope, inv.ID, "u1")
	if err != nil || got.ID != inv.ID {
		t.Fatalf("race loser got=%+v err=%v", got, err)
	}
	if h.sender.autoCalls != 0 || h.files.uploads != 0 || h.links.ensured != 0 {
		t.Fatalf("loser must not send/upload/link: send=%d pdf=%d link=%d", h.sender.autoCalls, h.files.uploads, h.links.ensured)
	}
}

func TestIssueSurvivesSnapshotFailure(t *testing.T) {
	h := newInvoiceHarness(t)
	h.files.err = errors.New("storage down")
	inv := h.draft(t, InvoiceInput{})
	got, err := h.svc.Issue(ctx, scope, inv.ID, "u1")
	if err != nil || got.Status != domain.InvoiceIssued || got.InvoiceNumber == "" {
		t.Fatalf("issue must survive pdf failure: %+v err=%v", got, err)
	}
	if h.sender.autoCalls != 1 {
		t.Fatalf("auto send still runs (it records the failure itself), calls = %d", h.sender.autoCalls)
	}
}

func TestIssueVoidInvoiceIsRejected(t *testing.T) {
	h := newInvoiceHarness(t)
	inv := h.draft(t, InvoiceInput{})
	_, _ = h.svc.Void(ctx, scope, inv.ID, "batal", "u1")
	if _, err := h.svc.Issue(ctx, scope, inv.ID, "u1"); err == nil {
		t.Fatal("issuing a void invoice must fail")
	}
	if _, err := h.svc.Issue(ctx, scope, "nope", "u1"); !errors.Is(err, receivable.ErrInvoiceNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}

func TestCreateAndIssueIdempotentBySource(t *testing.T) {
	h := newInvoiceHarness(t)
	in := InvoiceInput{AccountID: "a1", SourceType: domain.SourceSalesOrder, SourceID: "so1", IdempotencyKey: "initial", Lines: []LineInput{line("Paket", "100000")}}
	first, err := h.svc.CreateAndIssue(ctx, scope, in, "u1")
	if err != nil || first.Status != domain.InvoiceIssued {
		t.Fatalf("first = %+v err=%v", first, err)
	}
	second, err := h.svc.CreateAndIssue(ctx, scope, in, "u1")
	if err != nil || second.ID != first.ID {
		t.Fatalf("second = %+v err=%v", second, err)
	}
	if h.sender.autoCalls != 1 || h.counter.calls != 1 || len(h.store.invoices) != 1 {
		t.Fatalf("sends=%d numbers=%d invoices=%d", h.sender.autoCalls, h.counter.calls, len(h.store.invoices))
	}
}

func TestCreateAndIssueResumesInterruptedDraft(t *testing.T) {
	h := newInvoiceHarness(t)
	in := InvoiceInput{AccountID: "a1", SourceType: domain.SourceContract, SourceID: "c1", IdempotencyKey: "2026-10", Lines: []LineInput{line("Internet", "300000")}}
	draft, err := h.svc.CreateDraft(ctx, scope, in, "u1") // proses sebelumnya mati sebelum Issue
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.svc.CreateAndIssue(ctx, scope, in, "u1")
	if err != nil || got.ID != draft.ID || got.Status != domain.InvoiceIssued || h.sender.autoCalls != 1 {
		t.Fatalf("resume got=%+v err=%v sends=%d", got, err, h.sender.autoCalls)
	}
}

func TestCreateDraftValidationAndDefaults(t *testing.T) {
	h := newInvoiceHarness(t)
	bad := []InvoiceInput{
		{AccountID: "a1"}, // tanpa baris
		{AccountID: "a1", Lines: []LineInput{line("x", "1")}, Channels: []string{"sms"}},
		{AccountID: "a1", Lines: []LineInput{line(" ", "1")}},
		{AccountID: "a1", Lines: []LineInput{line("x", "-5")}},
		{AccountID: "a1", Lines: []LineInput{line("x", "1")}, PeriodStart: day("2026-10-10"), PeriodEnd: day("2026-10-01")},
		{AccountID: "a1", Lines: []LineInput{{LineInput: pricing.LineInput{Description: "x", UnitPrice: "1"}, PeriodStart: day("2026-10-10"), PeriodEnd: day("2026-10-01")}}},
		{AccountID: "a1", Lines: []LineInput{line("x", "1")}, SourceType: "bogus"},
		{AccountID: "a1", Lines: []LineInput{line("x", "1")}, Currency: "rupiah"},
	}
	for i, in := range bad {
		if _, err := h.svc.CreateDraft(ctx, scope, in, "u1"); !errors.Is(err, ErrInvalidInvoice) {
			t.Errorf("case %d: err = %v, want ErrInvalidInvoice", i, err)
		}
	}
	if _, err := h.svc.CreateDraft(ctx, scope, InvoiceInput{Lines: []LineInput{line("x", "1")}}, "u1"); !errors.Is(err, receivable.ErrAccountNotFound) && !errors.Is(err, ErrInvalidInvoice) {
		t.Fatalf("missing account id err = %v", err)
	}

	inv := h.draft(t, InvoiceInput{})
	if len(inv.Channels) != 1 || inv.Channels[0] != "email" || inv.Currency != "IDR" || inv.SourceType != domain.SourceManual {
		t.Fatalf("defaults = channels %v currency %q source %q", inv.Channels, inv.Currency, inv.SourceType)
	}
	dup := h.draft(t, InvoiceInput{Channels: []string{"whatsapp", "email", "whatsapp"}})
	if len(dup.Channels) != 2 {
		t.Fatalf("channels must be de-duplicated: %v", dup.Channels)
	}
}

func TestCreateDraftUnknownAccount(t *testing.T) {
	h := newInvoiceHarness(t)
	h.svc.accounts = fakeAccounts{missing: true}
	if _, err := h.svc.CreateDraft(ctx, scope, InvoiceInput{AccountID: "ghost", Lines: []LineInput{line("x", "1")}}, "u1"); !errors.Is(err, receivable.ErrAccountNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateDraftOnlyWhileDraft(t *testing.T) {
	h := newInvoiceHarness(t)
	inv := h.draft(t, InvoiceInput{})
	upd, err := h.svc.UpdateDraft(ctx, scope, inv.ID, InvoiceInput{AccountID: "a1", Notes: "revisi", Lines: []LineInput{line("Baru", "1000")}}, "u1")
	if err != nil || upd.Notes != "revisi" || upd.GrandTotal != "1000.00" {
		t.Fatalf("updated = %+v err=%v", upd, err)
	}
	_, _ = h.svc.Issue(ctx, scope, inv.ID, "u1")
	if _, err := h.svc.UpdateDraft(ctx, scope, inv.ID, InvoiceInput{AccountID: "a1", Lines: []LineInput{line("x", "1")}}, "u1"); err == nil {
		t.Fatal("issued invoice must not be editable")
	}
}

func TestVoidRequiresReasonAndRevokesLink(t *testing.T) {
	h := newInvoiceHarness(t)
	inv, _ := h.svc.Issue(ctx, scope, h.draft(t, InvoiceInput{}).ID, "u1")
	for _, reason := range []string{"", "   ", strings.Repeat("x", 501)} {
		if _, err := h.svc.Void(ctx, scope, inv.ID, reason, "u1"); !errors.Is(err, ErrVoidReasonRequired) {
			t.Errorf("reason %q: err = %v", reason[:min(len(reason), 5)], err)
		}
	}
	if h.links.revoked != 0 {
		t.Fatal("link must not be revoked by a rejected void")
	}
	got, err := h.svc.Void(ctx, scope, inv.ID, " salah input ", "u1")
	if err != nil || got.Status != domain.InvoiceVoid || got.VoidReason != "salah input" || h.links.revoked != 1 {
		t.Fatalf("void = %+v err=%v revoked=%d", got, err, h.links.revoked)
	}
	// snapshot dibuat ulang dengan stempel DIBATALKAN
	if last := h.renderer.docs[len(h.renderer.docs)-1]; last.Stamp != "DIBATALKAN" {
		t.Fatalf("last render stamp = %q", last.Stamp)
	}
}

func TestPDFDraftIsOnDemandAndSnapshotIsReused(t *testing.T) {
	h := newInvoiceHarness(t)
	draft := h.draft(t, InvoiceInput{})
	pdf, err := h.svc.PDF(ctx, scope, draft.ID)
	if err != nil || h.renderer.docs[0].Stamp != "DRAFT" || h.files.uploads != 0 || len(pdf.Content) == 0 {
		t.Fatalf("draft pdf: err=%v stamp=%q uploads=%d", err, h.renderer.docs[0].Stamp, h.files.uploads)
	}
	issued, _ := h.svc.Issue(ctx, scope, draft.ID, "u1")
	renders := len(h.renderer.docs)
	pdf, err = h.svc.PDF(ctx, scope, issued.ID)
	if err != nil || pdf.Filename != "INV-2026-0001.pdf" || len(h.renderer.docs) != renders {
		t.Fatalf("snapshot must be read, not re-rendered: err=%v file=%s renders %d→%d", err, pdf.Filename, renders, len(h.renderer.docs))
	}
}

func TestPDFCreatesMissingSnapshot(t *testing.T) {
	h := newInvoiceHarness(t)
	h.files.err = errors.New("storage down during issue")
	issued, _ := h.svc.Issue(ctx, scope, h.draft(t, InvoiceInput{}).ID, "u1")
	if issued.PDFAssetID != nil {
		t.Fatal("precondition: no snapshot")
	}
	h.files.err = nil
	if _, err := h.svc.PDF(ctx, scope, issued.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := h.svc.Get(ctx, scope, issued.ID); got.PDFAssetID == nil || h.files.uploads != 1 {
		t.Fatalf("snapshot must now exist: %+v uploads=%d", got.PDFAssetID, h.files.uploads)
	}
}

func TestLinkOnlyForSendableStatuses(t *testing.T) {
	h := newInvoiceHarness(t)
	draft := h.draft(t, InvoiceInput{})
	if _, _, err := h.svc.Link(ctx, scope, draft.ID, "u1"); !errors.Is(err, ErrInvoiceNotSendable) {
		t.Fatalf("draft link err = %v", err)
	}
	issued, _ := h.svc.Issue(ctx, scope, draft.ID, "u1")
	url, exp, err := h.svc.Link(ctx, scope, issued.ID, "u1")
	if err != nil || url != "https://app.test/i/tok" || exp.IsZero() {
		t.Fatalf("link = %q %v err=%v", url, exp, err)
	}
	_, _ = h.svc.Void(ctx, scope, issued.ID, "batal", "u1")
	if _, _, err := h.svc.Link(ctx, scope, issued.ID, "u1"); !errors.Is(err, ErrInvoiceNotSendable) {
		t.Fatalf("void link err = %v", err)
	}
}

func TestMarkOverdueDelegatesWithBusinessToday(t *testing.T) {
	h := newInvoiceHarness(t)
	n, err := MarkOverdue(ctx, scope, h.store, *day("2026-10-06"))
	if err != nil || n != 1 || h.store.overdueSeen.Format("2006-01-02") != "2026-10-06" {
		t.Fatalf("n=%d err=%v seen=%v", n, err, h.store.overdueSeen)
	}
}

func TestPICMustBeAnActiveMemberOfThisOrganization(t *testing.T) {
	h := newInvoiceHarness(t)
	h.svc.members = fakeMembers{byID: map[string]Member{"u-pic": {UserID: "u-pic"}}}
	in := InvoiceInput{AccountID: "a1", Lines: []LineInput{line("x", "1")}}

	in.PICUserID = "u-pic"
	if inv, err := h.svc.CreateDraft(ctx, scope, in, "u1"); err != nil || inv.PICUserID != "u-pic" {
		t.Fatalf("valid pic: %+v err=%v", inv, err)
	}
	in.PICUserID = "user-from-another-tenant"
	if _, err := h.svc.CreateDraft(ctx, scope, in, "u1"); !errors.Is(err, ErrInvalidInvoice) {
		t.Fatalf("foreign pic err = %v", err)
	}
	in.PICUserID = ""
	if _, err := h.svc.CreateDraft(ctx, scope, in, "u1"); err != nil {
		t.Fatalf("empty pic is allowed: %v", err)
	}
}

func TestValidationMessagesAreIndonesian(t *testing.T) {
	h := newInvoiceHarness(t)
	_, err := h.svc.CreateDraft(ctx, scope, InvoiceInput{AccountID: "a1"}, "u1")
	if err == nil || !strings.Contains(err.Error(), "baris") {
		t.Fatalf("message must be Indonesian, got %v", err)
	}
	_, err = h.svc.CreateDraft(ctx, scope, InvoiceInput{AccountID: "a1", Lines: []LineInput{line(" ", "1")}}, "u1")
	if err == nil || !strings.Contains(err.Error(), "deskripsi") {
		t.Fatalf("pricing error must be Indonesian, got %v", err)
	}
}

// Kolom uang adalah numeric(18,2): nilai yang meluap harus ditolak sebagai validasi (422),
// bukan lolos ke database dan menjadi 500.
func TestCreateDraftRejectsAmountsBeyondColumnRange(t *testing.T) {
	h := newInvoiceHarness(t)
	huge := "99999999999999999999" // 20 digit
	cases := map[string]LineInput{
		"unit price": line("x", huge),
		"quantity":   {LineInput: pricing.LineInput{Description: "x", Quantity: huge, UnitPrice: "1"}},
		"line total": {LineInput: pricing.LineInput{Description: "x", Quantity: "9999999999", UnitPrice: "9999999999"}},
	}
	for name, l := range cases {
		if _, err := h.svc.CreateDraft(ctx, scope, InvoiceInput{AccountID: "a1", Lines: []LineInput{l}}, "u1"); !errors.Is(err, ErrInvalidInvoice) {
			t.Errorf("%s: err = %v, want ErrInvalidInvoice", name, err)
		}
	}
	// grand total is the sum of lines: two lines that fit individually but not together
	two := []LineInput{line("a", "9000000000000000"), line("b", "9000000000000000")}
	if _, err := h.svc.CreateDraft(ctx, scope, InvoiceInput{AccountID: "a1", Lines: two}, "u1"); !errors.Is(err, ErrInvalidInvoice) {
		t.Errorf("sum overflow: err = %v, want ErrInvalidInvoice", err)
	}
	// the largest value the column can hold is still accepted
	if _, err := h.svc.CreateDraft(ctx, scope, InvoiceInput{AccountID: "a1", Lines: []LineInput{line("max", "9999999999999999.99")}}, "u1"); err != nil {
		t.Errorf("max value must be accepted: %v", err)
	}
}
