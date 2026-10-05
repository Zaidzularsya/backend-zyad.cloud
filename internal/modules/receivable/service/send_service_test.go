package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
)

// ---- fakes ----

type fakeSendRepo struct{ records []domain.Send }

func (f *fakeSendRepo) Record(_ context.Context, _ coretenant.Scope, p repository.RecordSendParams) (domain.Send, error) {
	if p.ClientRequestID != "" {
		for _, r := range f.records {
			if r.InvoiceID == p.InvoiceID && r.ClientRequestID == p.ClientRequestID {
				return domain.Send{}, errors.New("duplicate client_request_id")
			}
		}
	}
	s := domain.Send{ID: "s" + string(rune('0'+len(f.records)+1)), InvoiceID: p.InvoiceID, Channel: p.Channel, Recipient: p.Recipient,
		Status: p.Status, Error: p.Error, ExternalMessageID: p.ExternalMessageID, ClientRequestID: p.ClientRequestID, Trigger: p.Trigger, SentBy: p.SentBy}
	f.records = append(f.records, s)
	return s, nil
}
func (f *fakeSendRepo) FindByClientRequest(_ context.Context, _ coretenant.Scope, invoiceID, crid string) (domain.Send, error) {
	for _, r := range f.records {
		if r.InvoiceID == invoiceID && r.ClientRequestID == crid {
			return r, nil
		}
	}
	return domain.Send{}, pgx.ErrNoRows
}
func (f *fakeSendRepo) ListByInvoice(_ context.Context, _ coretenant.Scope, invoiceID string) ([]domain.Send, error) {
	out := []domain.Send{}
	for i := len(f.records) - 1; i >= 0; i-- {
		if f.records[i].InvoiceID == invoiceID {
			out = append(out, f.records[i])
		}
	}
	return out, nil
}
func (f *fakeSendRepo) LatestFailedByInvoice(context.Context, coretenant.Scope, string) (domain.Send, error) {
	return domain.Send{}, pgx.ErrNoRows
}

type fakeEmail struct {
	calls []EmailInput
	err   error
}

func (f *fakeEmail) SendInvoiceEmail(_ context.Context, _ coretenant.Scope, in EmailInput) (string, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return "", f.err
	}
	return "mail-1", nil
}

type fakeWA struct {
	calls []WhatsAppInput
	err   error
}

func (f *fakeWA) SendInvoiceWhatsApp(_ context.Context, _ coretenant.Scope, in WhatsAppInput) (string, string, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return "", "", f.err
	}
	return "wa-1", "6281234567890", nil
}

type fakeDocs struct {
	pdfErr  error
	pdfs    int
	linkErr error
}

func (f *fakeDocs) PDF(context.Context, coretenant.Scope, string) (InvoicePDF, error) {
	f.pdfs++
	if f.pdfErr != nil {
		return InvoicePDF{}, f.pdfErr
	}
	return InvoicePDF{Filename: "INV-2026-0001.pdf", Content: []byte("%PDF")}, nil
}
func (f *fakeDocs) Link(context.Context, coretenant.Scope, string, string) (string, time.Time, error) {
	if f.linkErr != nil {
		return "", time.Time{}, f.linkErr
	}
	return "https://app.test/i/tok", time.Time{}, nil
}

type fakeNotifier struct{ events []FailureEvent }

func (n *fakeNotifier) NotifySendFailed(_ context.Context, _ coretenant.Scope, e FailureEvent) {
	n.events = append(n.events, e)
}

type fakeMembers struct {
	byID       map[string]Member
	permission []Member
}

func (m fakeMembers) Find(_ context.Context, _ coretenant.Scope, id string) (Member, error) {
	if mem, ok := m.byID[id]; ok {
		return mem, nil
	}
	return Member{}, pgx.ErrNoRows
}
func (m fakeMembers) WithPermission(_ context.Context, _ coretenant.Scope, _ string) ([]Member, error) {
	return m.permission, nil
}

type fakeEntitlements struct{ online bool }

func (e fakeEntitlements) RequireFeature(_ context.Context, _ string, key string) error {
	if key == "receivable.online_payment" && !e.online {
		return errors.New("not entitled")
	}
	return nil
}

type fakeInvoiceReader struct{ inv map[string]domain.Invoice }

func (f fakeInvoiceReader) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Invoice, error) {
	if i, ok := f.inv[id]; ok {
		return i, nil
	}
	return domain.Invoice{}, pgx.ErrNoRows
}

type sendHarness struct {
	svc      SendService
	sends    *fakeSendRepo
	email    *fakeEmail
	wa       *fakeWA
	docs     *fakeDocs
	notifier *fakeNotifier
	settings *fakeSettings
	members  fakeMembers
	reader   fakeInvoiceReader
	invoice  domain.Invoice
}

// newSendHarness: invoice issued (333.000, jatuh tempo 12 Okt), channels [email, whatsapp],
// account tanpa kontak CRM, PIC u-pic.
func newSendHarness(t *testing.T) *sendHarness {
	t.Helper()
	due := day("2026-10-12")
	h := &sendHarness{
		sends: &fakeSendRepo{}, email: &fakeEmail{}, wa: &fakeWA{}, docs: &fakeDocs{}, notifier: &fakeNotifier{},
		settings: &fakeSettings{domain.Settings{PaymentTermsDays: 7, DefaultChannels: []string{"email"}}},
		members:  fakeMembers{byID: map[string]Member{"u-pic": {UserID: "u-pic", Name: "Sari", Email: "sari@example.com"}}},
		invoice: domain.Invoice{
			ID: "inv-1", InvoiceNumber: "INV-2026-0001", Status: domain.InvoiceIssued, DueDate: due, Currency: "IDR",
			GrandTotal: "333000.00", AmountPaid: "0.00", Channels: []string{"email", "whatsapp"}, PICUserID: "u-pic",
			Account: domain.Account{ID: "a1", Name: "Budi", Email: "budi@example.com", Phone: "6281234567890"},
		},
	}
	h.reader = fakeInvoiceReader{inv: map[string]domain.Invoice{"inv-1": h.invoice}}
	h.rebuild(false)
	return h
}

func (h *sendHarness) rebuild(online bool) {
	h.reader.inv["inv-1"] = h.invoice
	h.svc = NewSendService(SendDeps{
		Invoices: h.reader, Sends: h.sends, Settings: h.settings, Docs: h.docs, Issuers: fakeIssuer{},
		Email: h.email, WhatsApp: h.wa, Notifier: h.notifier, Members: h.members, Entitlements: fakeEntitlements{online: online},
	})
}

// ---- tests ----

func TestAutoSendRecordsEachChannelAndNotifiesOnFailure(t *testing.T) {
	h := newSendHarness(t)
	h.svc.AutoSend(ctx, scope, h.invoice, "")
	sends := h.sends.records
	if len(sends) != 2 || sends[0].Status != "sent" || sends[1].Status != "failed" ||
		sends[1].Error != "WhatsApp hanya tersedia untuk pelanggan yang terhubung ke kontak CRM." || sends[1].Trigger != "auto" || sends[0].Trigger != "auto" {
		t.Fatalf("sends=%+v", sends)
	}
	if len(h.email.calls) != 1 || len(h.wa.calls) != 0 {
		t.Fatalf("email=%d wa=%d", len(h.email.calls), len(h.wa.calls))
	}
	if len(h.notifier.events) != 1 || h.notifier.events[0].UserID != "u-pic" || h.notifier.events[0].Channel != "WhatsApp" ||
		h.notifier.events[0].InvoiceNumber != "INV-2026-0001" || h.notifier.events[0].Reason != sends[1].Error {
		t.Fatalf("notify=%+v", h.notifier.events)
	}
	if h.email.calls[0].ClientRequestID == "" || h.email.calls[0].SenderUserID != "u-pic" || h.email.calls[0].To != "budi@example.com" {
		t.Fatalf("email input = %+v", h.email.calls[0])
	}
}

func TestAutoSendDeliveryFailureHidesProviderDetail(t *testing.T) {
	h := newSendHarness(t)
	h.email.err = errors.New("smtp 550 5.7.1 relay denied for secret-host.internal")
	h.invoice.Channels = []string{"email"}
	h.rebuild(false)
	h.svc.AutoSend(ctx, scope, h.invoice, "")
	if len(h.sends.records) != 1 || h.sends.records[0].Status != "failed" || strings.Contains(h.sends.records[0].Error, "550") || strings.Contains(h.sends.records[0].Error, "secret") {
		t.Fatalf("records = %+v", h.sends.records)
	}
	if h.sends.records[0].Error == "" || len(h.notifier.events) != 1 {
		t.Fatalf("a readable reason is required and PIC must be notified: %+v / %+v", h.sends.records, h.notifier.events)
	}
}

func TestManualSendIdempotentAndChannelErrorsNotLogged(t *testing.T) {
	h := newSendHarness(t)
	first, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email", ClientRequestID: "r1"}, "u-actor")
	if err != nil || first.Status != "sent" || first.Trigger != "manual" || first.SentBy != "u-actor" || first.Recipient != "budi@example.com" {
		t.Fatalf("first = %+v err=%v", first, err)
	}
	again, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email", ClientRequestID: "r1"}, "u-actor")
	if err != nil || again.ID != first.ID || len(h.email.calls) != 1 || len(h.sends.records) != 1 {
		t.Fatalf("retry must return the same record: again=%+v calls=%d records=%d err=%v", again, len(h.email.calls), len(h.sends.records), err)
	}

	before := len(h.sends.records)
	if _, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "whatsapp"}, "u-actor"); !errors.Is(err, ErrChannelUnavailable) {
		t.Fatalf("whatsapp without contact err = %v", err)
	}
	if len(h.sends.records) != before || len(h.notifier.events) != 0 {
		t.Fatal("channel errors must not be logged nor notified in manual mode")
	}
	if _, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "sms"}, "u-actor"); !errors.Is(err, ErrInvalidInvoice) {
		t.Fatalf("unknown channel err = %v", err)
	}
	if _, err := h.svc.Send(ctx, scope, "nope", SendInput{Channel: "email"}, "u-actor"); !errors.Is(err, receivable.ErrInvoiceNotFound) {
		t.Fatalf("missing invoice err = %v", err)
	}
	for _, st := range []domain.InvoiceStatus{domain.InvoiceDraft, domain.InvoiceVoid} {
		h.invoice.Status = st
		h.rebuild(false)
		if _, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email"}, "u-actor"); !errors.Is(err, ErrInvoiceNotSendable) {
			t.Fatalf("status %s err = %v", st, err)
		}
	}
	h.invoice.Status = domain.InvoicePaid // bukti lunas boleh dikirim
	h.rebuild(false)
	if _, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email"}, "u-actor"); err != nil {
		t.Fatalf("paid invoice must be sendable: %v", err)
	}
}

func TestManualSendWhatsAppUsesLinkedContact(t *testing.T) {
	h := newSendHarness(t)
	h.invoice.Account.SourceType, h.invoice.Account.SourceID = "crm_contact", "contact-9"
	h.rebuild(false)
	s, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "whatsapp"}, "u-actor")
	if err != nil || s.Status != "sent" || s.Recipient != "6281234567890" || s.ExternalMessageID != "wa-1" {
		t.Fatalf("send = %+v err=%v", s, err)
	}
	if got := h.wa.calls[0]; got.ContactID != "contact-9" || got.SenderUserID != "u-pic" || got.Attachment == nil || !strings.Contains(got.Text, "https://app.test/i/tok") {
		t.Fatalf("wa input = %+v", got)
	}
}

func TestManualSendDeliveryFailureIsRecordedNotReturned(t *testing.T) {
	h := newSendHarness(t)
	h.email.err = errors.New("connection reset by peer")
	s, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email"}, "u-actor")
	if err != nil || s.Status != "failed" || s.Error == "" || strings.Contains(s.Error, "reset") {
		t.Fatalf("send = %+v err=%v", s, err)
	}
	if len(h.notifier.events) != 0 {
		t.Fatal("manual failures are shown to the user directly, no notification")
	}
}

func TestSendWithoutPDFRecordsFailure(t *testing.T) {
	h := newSendHarness(t)
	h.docs.pdfErr = errors.New("storage down")
	s, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email"}, "u-actor")
	if err != nil || s.Status != "failed" || s.Error != "PDF invoice belum tersedia. Coba lagi." || len(h.email.calls) != 0 {
		t.Fatalf("send = %+v err=%v calls=%d", s, err, len(h.email.calls))
	}
}

func TestSenderFallback(t *testing.T) {
	h := newSendHarness(t)
	h.invoice.PICUserID = ""
	h.invoice.Channels = []string{"email"}
	h.rebuild(false)

	// manual tanpa PIC → pengguna yang menekan tombol
	if _, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email"}, "u-actor"); err != nil {
		t.Fatal(err)
	}
	if got := h.email.calls[0].SenderUserID; got != "u-actor" {
		t.Fatalf("manual sender = %q, want u-actor", got)
	}

	// otomatis tanpa PIC → pengirim default
	h.settings.s.DefaultSenderUserID = "u-def"
	h.svc.AutoSend(ctx, scope, h.invoice, "u-actor")
	if got := h.email.calls[1].SenderUserID; got != "u-def" {
		t.Fatalf("auto sender = %q, want u-def", got)
	}

	// otomatis tanpa PIC dan tanpa default → gagal tercatat
	h.settings.s.DefaultSenderUserID = ""
	h.sends.records = nil
	h.svc.AutoSend(ctx, scope, h.invoice, "")
	if len(h.sends.records) != 1 || h.sends.records[0].Status != "failed" || !strings.HasPrefix(h.sends.records[0].Error, "Belum ada pengirim.") || len(h.email.calls) != 2 {
		t.Fatalf("records=%+v emailCalls=%d", h.sends.records, len(h.email.calls))
	}

	// manual: tanpa PIC, tanpa default, actor kosong → ChannelError
	if _, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email"}, ""); !errors.Is(err, ErrChannelUnavailable) {
		t.Fatalf("no sender err = %v", err)
	}
}

func TestNotifyFallsBackToPermissionHolders(t *testing.T) {
	h := newSendHarness(t)
	h.invoice.PICUserID = ""
	h.invoice.Channels = []string{"email"}
	h.members.permission = []Member{{UserID: "u-owner", Name: "Owner", Email: "o@example.com"}, {UserID: "u-fin", Name: "Fin", Email: "f@example.com"}}
	h.rebuild(false)
	h.svc.AutoSend(ctx, scope, h.invoice, "") // tanpa PIC & tanpa default → gagal
	if len(h.notifier.events) != 2 || h.notifier.events[0].UserID != "u-owner" || h.notifier.events[1].UserID != "u-fin" {
		t.Fatalf("events = %+v", h.notifier.events)
	}
}

func TestInvoiceMessageEscapesAndLinks(t *testing.T) {
	h := newSendHarness(t)
	h.invoice.Account.Name = "<b>Budi</b>"
	h.invoice.PeriodStart, h.invoice.PeriodEnd = day("2026-10-01"), day("2026-10-31")
	h.rebuild(false)
	if _, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email", Message: "Halo <script>x</script>"}, "u-actor"); err != nil {
		t.Fatal(err)
	}
	in := h.email.calls[0]
	if strings.Contains(in.BodyHTML, "<b>Budi</b>") || strings.Contains(in.BodyHTML, "<script>") || !strings.Contains(in.BodyHTML, "&lt;script&gt;") {
		t.Fatalf("html must be escaped: %s", in.BodyHTML)
	}
	if !strings.Contains(in.BodyHTML, "Lihat invoice: ") || !strings.Contains(in.BodyHTML, "https://app.test/i/tok") || strings.Contains(in.BodyHTML, "Lihat &amp; bayar") {
		t.Fatalf("link line = %s", in.BodyHTML)
	}
	if in.Subject != "Invoice INV-2026-0001 – PT Zyad Technovation" || in.Attachment == nil || in.Attachment.MimeType != "application/pdf" || in.Attachment.Filename != "INV-2026-0001.pdf" {
		t.Fatalf("subject=%q attachment=%+v", in.Subject, in.Attachment)
	}

	// default opening + periode + tautan bayar bila online_payment aktif
	h.rebuild(true)
	if _, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email"}, "u-actor"); err != nil {
		t.Fatal(err)
	}
	body := h.email.calls[1].BodyHTML
	for _, want := range []string{"sebesar Rp 333.000", "jatuh tempo 12 Okt 2026", "Periode: 1 Okt 2026 – 31 Okt 2026", "Lihat &amp; bayar: "} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestMessageOmitsLinkLineWhenLinkUnavailable(t *testing.T) {
	h := newSendHarness(t)
	h.docs.linkErr = errors.New("links down")
	if _, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email"}, "u-actor"); err != nil {
		t.Fatalf("a missing link must not block sending: %v", err)
	}
	if strings.Contains(h.email.calls[0].BodyHTML, "Lihat") {
		t.Fatalf("no link line expected: %s", h.email.calls[0].BodyHTML)
	}
}

func TestEmailRecipientOverrideAndValidation(t *testing.T) {
	h := newSendHarness(t)
	if s, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email", Recipient: "finance@pt-maju.id"}, "u-actor"); err != nil || s.Recipient != "finance@pt-maju.id" {
		t.Fatalf("override = %+v err=%v", s, err)
	}
	if _, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email", Recipient: "bukan email"}, "u-actor"); !errors.Is(err, ErrChannelUnavailable) {
		t.Fatalf("invalid recipient err = %v", err)
	}
	h.invoice.Account.Email = ""
	h.rebuild(false)
	if _, err := h.svc.Send(ctx, scope, "inv-1", SendInput{Channel: "email"}, "u-actor"); !errors.Is(err, ErrChannelUnavailable) {
		t.Fatalf("account without email err = %v", err)
	}
}
