package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
)

const (
	channelEmail    = "email"
	channelWhatsApp = "whatsapp"

	reasonNoSender        = "Belum ada pengirim. Atur PIC atau pengirim default di Pengaturan Penagihan."
	reasonWANoContact     = "WhatsApp hanya tersedia untuk pelanggan yang terhubung ke kontak CRM."
	reasonPDFMissing      = "PDF invoice belum tersedia. Coba lagi."
	reasonSendFailed      = "Pengiriman gagal. Coba lagi nanti."
	permissionInvoiceSend = "invoice.send"
)

type SendInput struct{ Channel, Recipient, Message, ClientRequestID string }

type SendService interface {
	// Send (manual): kanal tak tersedia → *ChannelError (tidak dicatat); gagal kirim → dicatat failed, error nil.
	Send(ctx context.Context, scope coretenant.Scope, invoiceID string, in SendInput, userID string) (domain.Send, error)
	// AutoSend: semua inv.Channels; setiap kanal dicatat (sent/failed, trigger auto, termasuk ChannelError sebagai
	// failed); setiap failed → notifikasi ke PIC, atau ke pemegang permission invoice.send bila PIC kosong.
	AutoSend(ctx context.Context, scope coretenant.Scope, inv domain.Invoice, actorUserID string)
	List(ctx context.Context, scope coretenant.Scope, invoiceID string) ([]domain.Send, error)
	// LatestByInvoices: kiriman terbaru per invoice (untuk kolom "kiriman terakhir" di daftar); tanpa kiriman → tidak ada key.
	LatestByInvoices(ctx context.Context, scope coretenant.Scope, invoiceIDs []string) (map[string]domain.Send, error)
}

// FailureEvent: satu notifikasi "invoice gagal dikirim" untuk satu penerima.
type FailureEvent struct {
	UserID, UserName, UserEmail                          string
	InvoiceID, InvoiceNumber, Recipient, Channel, Reason string // Channel = label ("Email" | "WhatsApp")
}

type SendNotifier interface {
	NotifySendFailed(ctx context.Context, scope coretenant.Scope, e FailureEvent)
}

type Member struct{ UserID, Name, Email string }

// MemberDirectory: anggota aktif organisasi (implementasi di internal/app; receivable tidak mengimpor CRM).
type MemberDirectory interface {
	Find(ctx context.Context, scope coretenant.Scope, userID string) (Member, error)
	WithPermission(ctx context.Context, scope coretenant.Scope, permission string) ([]Member, error)
	// Active: semua anggota aktif (pilihan pengirim default / PIC).
	Active(ctx context.Context, scope coretenant.Scope) ([]Member, error)
}

// InvoiceReader dan InvoiceDocs adalah subset repository/InvoiceService yang dipakai pengiriman.
type InvoiceReader interface {
	FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Invoice, error)
}

type InvoiceDocs interface {
	PDF(ctx context.Context, scope coretenant.Scope, id string) (InvoicePDF, error)
	Link(ctx context.Context, scope coretenant.Scope, id, userID string) (string, time.Time, error)
}

// Entitlements: checker feature organisasi (receivable.online_payment menentukan teks "Lihat & bayar").
type Entitlements interface {
	RequireFeature(ctx context.Context, organizationID, featureKey string) error
}

type SendDeps struct {
	Invoices     InvoiceReader
	Sends        repository.SendRepository
	Settings     repository.SettingsRepository
	Docs         InvoiceDocs
	Issuers      IssuerSource
	Email        EmailSender    // nil = kanal tidak tersedia
	WhatsApp     WhatsAppSender // nil = kanal tidak tersedia
	Notifier     SendNotifier   // nil = tanpa notifikasi
	Members      MemberDirectory
	Entitlements Entitlements
}

type sendService struct{ SendDeps }

func NewSendService(d SendDeps) SendService { return &sendService{d} }

func (s *sendService) List(ctx context.Context, scope coretenant.Scope, invoiceID string) ([]domain.Send, error) {
	return s.Sends.ListByInvoice(ctx, scope, invoiceID)
}

func (s *sendService) LatestByInvoices(ctx context.Context, scope coretenant.Scope, invoiceIDs []string) (map[string]domain.Send, error) {
	return s.Sends.LatestByInvoices(ctx, scope, invoiceIDs)
}

// attempt adalah hasil satu percobaan kirim pada satu kanal.
type attempt struct {
	recipient, externalID string
	unavailable           string // kanal tak bisa dipakai (alasan aman ditampilkan)
	failure               string // gagal kirim (alasan aman ditampilkan)
}

func (a attempt) ok() bool { return a.unavailable == "" && a.failure == "" }

func (a attempt) reason() string {
	if a.unavailable != "" {
		return a.unavailable
	}
	return a.failure
}

func sendable(st domain.InvoiceStatus) bool {
	return st == domain.InvoiceIssued || st == domain.InvoiceOverdue || st == domain.InvoicePaid
}

func (s *sendService) Send(ctx context.Context, scope coretenant.Scope, invoiceID string, in SendInput, userID string) (domain.Send, error) {
	channel := strings.TrimSpace(in.Channel)
	if channel != channelEmail && channel != channelWhatsApp {
		return domain.Send{}, invalidInvoice("Kanal harus email atau whatsapp.")
	}
	if in.ClientRequestID != "" {
		if prev, err := s.Sends.FindByClientRequest(ctx, scope, invoiceID, in.ClientRequestID); err == nil {
			return prev, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return domain.Send{}, err
		}
	}
	inv, err := s.Invoices.FindByID(ctx, scope, invoiceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Send{}, receivable.ErrInvoiceNotFound
	}
	if err != nil {
		return domain.Send{}, err
	}
	if !sendable(inv.Status) {
		return domain.Send{}, ErrInvoiceNotSendable
	}
	settings, err := s.Settings.Get(ctx, scope)
	if err != nil {
		return domain.Send{}, err
	}
	sender := firstNonEmpty(inv.PICUserID, userID, settings.DefaultSenderUserID)

	res := s.attempt(ctx, scope, inv, channel, in, sender, in.ClientRequestID)
	if res.unavailable != "" {
		return domain.Send{}, &ChannelError{Reason: res.unavailable}
	}
	rec, err := s.record(ctx, scope, inv, channel, res, "manual", userID, in.ClientRequestID)
	if err != nil && in.ClientRequestID != "" { // balapan pada client_request_id yang sama
		if prev, findErr := s.Sends.FindByClientRequest(ctx, scope, invoiceID, in.ClientRequestID); findErr == nil {
			return prev, nil
		}
	}
	return rec, err
}

func (s *sendService) AutoSend(ctx context.Context, scope coretenant.Scope, inv domain.Invoice, actorUserID string) {
	settings, err := s.Settings.Get(ctx, scope)
	if err != nil {
		slog.WarnContext(ctx, "receivable: auto send skipped, settings unavailable", "invoice_id", inv.ID, "error", err)
		return
	}
	sender := firstNonEmpty(inv.PICUserID, settings.DefaultSenderUserID)
	for _, channel := range inv.Channels {
		// ID deterministik per invoice+kanal: pengiriman otomatis tidak tercatat dua kali.
		requestID := fmt.Sprintf("auto-%s-%s", inv.ID, channel)
		res := s.attempt(ctx, scope, inv, channel, SendInput{Channel: channel}, sender, requestID)
		if _, err := s.record(ctx, scope, inv, channel, res, "auto", actorUserID, requestID); err != nil {
			slog.WarnContext(ctx, "receivable: record auto send failed", "invoice_id", inv.ID, "channel", channel, "error", err)
			continue
		}
		if !res.ok() {
			s.notifyFailure(ctx, scope, inv, channel, res)
		}
	}
}

// attempt memeriksa ketersediaan kanal lalu mengirim. Tidak menulis log kiriman.
func (s *sendService) attempt(ctx context.Context, scope coretenant.Scope, inv domain.Invoice, channel string, in SendInput, senderUserID, requestID string) attempt {
	switch channel {
	case channelEmail:
		return s.attemptEmail(ctx, scope, inv, in, senderUserID, requestID)
	case channelWhatsApp:
		return s.attemptWhatsApp(ctx, scope, inv, in, senderUserID)
	}
	return attempt{unavailable: "Kanal tidak dikenal."}
}

func (s *sendService) attemptEmail(ctx context.Context, scope coretenant.Scope, inv domain.Invoice, in SendInput, senderUserID, requestID string) attempt {
	to := strings.TrimSpace(in.Recipient)
	if to == "" {
		to = strings.TrimSpace(inv.Account.Email)
	}
	res := attempt{recipient: to}
	switch {
	case s.Email == nil:
		res.unavailable = "Email belum dikonfigurasi."
		return res
	case to == "":
		res.unavailable = "Pelanggan belum punya email."
		return res
	case !validEmail(to):
		res.unavailable = "Alamat email penerima tidak valid."
		return res
	case senderUserID == "":
		res.unavailable = reasonNoSender
		return res
	}
	msg, failure := s.compose(ctx, scope, inv, in.Message, senderUserID)
	if failure != "" {
		res.failure = failure
		return res
	}
	id, err := s.Email.SendInvoiceEmail(ctx, scope, EmailInput{
		SenderUserID: senderUserID, To: to, Subject: msg.subject, BodyHTML: msg.html, ContactID: inv.Account.ContactID(),
		ClientRequestID: requestID, Attachment: msg.attachment,
	})
	return s.finish(ctx, inv, res, id, err)
}

func (s *sendService) attemptWhatsApp(ctx context.Context, scope coretenant.Scope, inv domain.Invoice, in SendInput, senderUserID string) attempt {
	res := attempt{recipient: inv.Account.Phone}
	switch {
	case s.WhatsApp == nil:
		res.unavailable = "WhatsApp belum dikonfigurasi."
		return res
	case inv.Account.ContactID() == "":
		res.unavailable = reasonWANoContact
		return res
	case senderUserID == "":
		res.unavailable = reasonNoSender
		return res
	}
	msg, failure := s.compose(ctx, scope, inv, in.Message, senderUserID)
	if failure != "" {
		res.failure = failure
		return res
	}
	id, recipient, err := s.WhatsApp.SendInvoiceWhatsApp(ctx, scope, WhatsAppInput{
		SenderUserID: senderUserID, ContactID: inv.Account.ContactID(), Text: msg.text, Attachment: msg.attachment,
	})
	if recipient != "" {
		res.recipient = recipient
	}
	return s.finish(ctx, inv, res, id, err)
}

// finish memetakan hasil adapter. Detail provider tidak pernah sampai ke pengguna; hanya log server.
func (s *sendService) finish(ctx context.Context, inv domain.Invoice, res attempt, externalID string, err error) attempt {
	var channelErr *ChannelError
	switch {
	case err == nil:
		res.externalID = externalID
	case errors.As(err, &channelErr):
		res.unavailable = channelErr.Reason
	default:
		slog.WarnContext(ctx, "receivable: invoice delivery failed", "invoice_id", inv.ID, "error", err)
		res.failure = reasonSendFailed
	}
	return res
}

func (s *sendService) record(ctx context.Context, scope coretenant.Scope, inv domain.Invoice, channel string, res attempt, trigger, sentBy, requestID string) (domain.Send, error) {
	status := "sent"
	if !res.ok() {
		status = "failed"
	}
	return s.Sends.Record(ctx, scope, repository.RecordSendParams{
		InvoiceID: inv.ID, Channel: channel, Recipient: res.recipient, Status: status, Error: res.reason(),
		ExternalMessageID: res.externalID, ClientRequestID: requestID, Trigger: trigger, SentBy: sentBy,
	})
}

func (s *sendService) notifyFailure(ctx context.Context, scope coretenant.Scope, inv domain.Invoice, channel string, res attempt) {
	if s.Notifier == nil || s.Members == nil {
		return
	}
	var recipients []Member
	if inv.PICUserID != "" {
		if m, err := s.Members.Find(ctx, scope, inv.PICUserID); err == nil {
			recipients = []Member{m}
		}
	}
	if len(recipients) == 0 {
		list, err := s.Members.WithPermission(ctx, scope, permissionInvoiceSend)
		if err != nil {
			slog.WarnContext(ctx, "receivable: cannot resolve failure recipients", "invoice_id", inv.ID, "error", err)
			return
		}
		recipients = list
	}
	for _, m := range recipients {
		s.Notifier.NotifySendFailed(ctx, scope, FailureEvent{
			UserID: m.UserID, UserName: m.Name, UserEmail: m.Email, InvoiceID: inv.ID, InvoiceNumber: inv.InvoiceNumber,
			Recipient: res.recipient, Channel: channelLabel(channel), Reason: res.reason(),
		})
	}
}

func channelLabel(channel string) string {
	if channel == channelWhatsApp {
		return "WhatsApp"
	}
	return "Email"
}

func validEmail(v string) bool {
	addr, err := mail.ParseAddress(v)
	return err == nil && addr.Address == v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// SendService memenuhi AutoSender yang dipakai InvoiceService.Issue.
var _ AutoSender = SendService(nil)
