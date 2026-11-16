package service

import (
	"errors"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/receivable/domain"
)

func arrearsRows() []ArrearsMessageRow {
	return []ArrearsMessageRow{
		{Number: "INV-2026-0101", Period: "5 Jul 2026 – 4 Agu 2026", Amount: "Rp 333.000", DueDate: "12 Juli 2026", Link: "https://app.test/i/a"},
		{Number: "INV-2026-0102", Period: "5 Agu 2026 – 4 Sep 2026", Amount: "Rp 250.000", DueDate: "12 Agustus 2026", Link: "https://app.test/i/b"},
	}
}

func TestBuildArrearsMessage_ListsEveryRowTotalAndLinks(t *testing.T) {
	subject, text, body := buildArrearsMessage("Budi", "PT Net", arrearsRows(), "Rp 583.000")
	if !strings.Contains(subject, "PT Net") || !strings.Contains(subject, "Tunggakan") {
		t.Fatalf("subject = %q", subject)
	}
	for _, want := range []string{"Budi", "INV-2026-0101", "5 Jul 2026 – 4 Agu 2026", "Rp 333.000", "12 Juli 2026",
		"https://app.test/i/a", "INV-2026-0102", "https://app.test/i/b", "Rp 583.000"} {
		if !strings.Contains(text, want) || !strings.Contains(body, want) {
			t.Fatalf("message missing %q\ntext=%s\nhtml=%s", want, text, body)
		}
	}
	if strings.Count(text, "https://app.test/i/") != 2 {
		t.Fatalf("want one link per invoice: %s", text)
	}
}

func TestBuildArrearsMessage_EscapesHTML(t *testing.T) {
	rows := arrearsRows()
	rows[0].Period = `<script>x</script>`
	_, _, body := buildArrearsMessage(`Budi "<b>"`, "PT <Net>", rows, "Rp 1")
	if strings.Contains(body, "<script>") || strings.Contains(body, "<b>") || strings.Contains(body, "PT <Net>") {
		t.Fatalf("html not escaped: %s", body)
	}
}

func TestBuildArrearsMessage_RowWithoutLink(t *testing.T) {
	rows := arrearsRows()
	rows[1].Link = ""
	_, text, _ := buildArrearsMessage("Budi", "", rows, "Rp 583.000")
	if strings.Count(text, "https://") != 1 {
		t.Fatalf("text = %s", text)
	}
}

func arrearsInvoices(h *sendHarness) []domain.Invoice {
	a, b := h.invoice, h.invoice
	a.ID, a.InvoiceNumber, a.PeriodStart, a.PeriodEnd = "inv-1", "INV-2026-0101", day("2026-07-05"), day("2026-08-04")
	b.ID, b.InvoiceNumber, b.PeriodStart, b.PeriodEnd, b.GrandTotal = "inv-2", "INV-2026-0102", day("2026-08-05"), day("2026-09-04"), "250000.00"
	return []domain.Invoice{a, b}
}

func TestNotifyArrears_EmailSent(t *testing.T) {
	h := newSendHarness(t)
	h.settings.s.DefaultSenderUserID = "u-sender"
	invs := arrearsInvoices(h)
	for i := range invs {
		invs[i].Channels = []string{"email"}
	}
	status, errText := h.svc.(*sendService).NotifyArrears(ctx, scope, domain.ArrearsRecording{ID: "rec-1"}, invs, "u-actor")
	if status != domain.ArrearsNotifySent || errText != "" {
		t.Fatalf("status=%s err=%s", status, errText)
	}
	if len(h.email.calls) != 1 || h.email.calls[0].To != "budi@example.com" || h.email.calls[0].Attachment != nil ||
		h.email.calls[0].SenderUserID != "u-sender" || !strings.Contains(h.email.calls[0].BodyHTML, "INV-2026-0102") ||
		!strings.Contains(h.email.calls[0].BodyHTML, "Rp 583.000") {
		t.Fatalf("email calls = %+v", h.email.calls)
	}
	if h.docs.pdfs != 0 {
		t.Fatalf("summary must not render PDFs, got %d", h.docs.pdfs)
	}
}

func TestNotifyArrears_NoContactIsFailedNotError(t *testing.T) {
	h := newSendHarness(t)
	invs := arrearsInvoices(h)
	for i := range invs {
		invs[i].Account.Email = ""
		invs[i].Channels = []string{"email", "whatsapp"}
	}
	status, errText := h.svc.(*sendService).NotifyArrears(ctx, scope, domain.ArrearsRecording{ID: "rec-1"}, invs, "u-actor")
	if status != domain.ArrearsNotifyFailed || !strings.Contains(errText, "email") || len(h.email.calls) != 0 || len(h.wa.calls) != 0 {
		t.Fatalf("status=%s err=%q email=%d wa=%d", status, errText, len(h.email.calls), len(h.wa.calls))
	}
}

func TestNotifyArrears_OneChannelFailsOtherSent(t *testing.T) {
	h := newSendHarness(t)
	invs := arrearsInvoices(h)
	for i := range invs {
		invs[i].Account.SourceType, invs[i].Account.SourceID = domain.SourceTypeCRMContact, "contact-1"
		invs[i].Channels = []string{"email", "whatsapp"}
	}
	h.email.err = errors.New("smtp down")
	status, errText := h.svc.(*sendService).NotifyArrears(ctx, scope, domain.ArrearsRecording{ID: "rec-1"}, invs, "u-actor")
	if status != domain.ArrearsNotifySent || !strings.Contains(errText, "Email") || len(h.wa.calls) != 1 ||
		!strings.Contains(h.wa.calls[0].Text, "INV-2026-0101") {
		t.Fatalf("status=%s err=%q wa=%+v", status, errText, h.wa.calls)
	}
}
