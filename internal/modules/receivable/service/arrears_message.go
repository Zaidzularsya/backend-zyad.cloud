package service

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"math/big"
	"strings"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/shared/docpdf"
)

// ArrearsMessageRow is one invoice line of the arrears summary (values already formatted).
type ArrearsMessageRow struct {
	Number, Period, Amount, DueDate, Link string
}

// buildArrearsMessage builds the one summary message of a recording: subject, plain text
// (WhatsApp) and HTML (email, every value escaped). Rows without a link list no URL.
func buildArrearsMessage(accountName, tenantName string, rows []ArrearsMessageRow, total string) (subject, text, htmlBody string) {
	subject = "Rincian Tunggakan"
	if tenantName != "" {
		subject += " – " + tenantName
	}
	greeting := "Halo"
	if strings.TrimSpace(accountName) != "" {
		greeting += " " + strings.TrimSpace(accountName)
	}
	opening := greeting + ", berikut rincian tunggakan yang kami catat. Setiap invoice bisa dibayar lewat tautannya."

	var t, b strings.Builder
	t.WriteString(opening + "\n")
	b.WriteString("<p>" + html.EscapeString(opening) + "</p><ol>")
	for i, r := range rows {
		line := fmt.Sprintf("%s · periode %s · %s · jatuh tempo %s", r.Number, r.Period, r.Amount, r.DueDate)
		fmt.Fprintf(&t, "\n%d. %s", i+1, line)
		b.WriteString("<li>" + html.EscapeString(line))
		if r.Link != "" {
			t.WriteString("\n   Bayar: " + r.Link)
			b.WriteString(`<br>Bayar: <a href="` + html.EscapeString(r.Link) + `">` + html.EscapeString(r.Link) + "</a>")
		}
		b.WriteString("</li>")
	}
	t.WriteString("\n\nTotal tunggakan: " + total)
	b.WriteString("</ol><p>Total tunggakan: <strong>" + html.EscapeString(total) + "</strong></p>")
	return subject, t.String(), b.String()
}

// SendArrearsSummary sends one summary over the invoices' channels (no attachment) and reports
// sent when at least one channel succeeded, failed otherwise; errText lists failed channels.
func (s *sendService) SendArrearsSummary(ctx context.Context, scope coretenant.Scope, acc domain.Account, channels []string,
	rows []ArrearsMessageRow, total, senderUserID, requestID string) (status, errText string) {
	tenantName := ""
	if s.Issuers != nil {
		if party, err := s.Issuers.OrganizationProfile(ctx, scope); err == nil {
			tenantName = party.Name
		}
	}
	subject, text, body := buildArrearsMessage(acc.Name, tenantName, rows, total)
	sent := false
	failures := []string{}
	for _, channel := range channels {
		reason := ""
		switch channel {
		case channelEmail:
			to := strings.TrimSpace(acc.Email)
			switch {
			case s.Email == nil:
				reason = "Email belum dikonfigurasi."
			case to == "":
				reason = "Pelanggan belum punya email."
			case !validEmail(to):
				reason = "Alamat email penerima tidak valid."
			case senderUserID == "":
				reason = reasonNoSender
			default:
				if _, err := s.Email.SendInvoiceEmail(ctx, scope, EmailInput{
					SenderUserID: senderUserID, To: to, Subject: subject, BodyHTML: body, ContactID: acc.ContactID(),
					ClientRequestID: requestID + "-email",
				}); err != nil {
					reason = s.deliveryReason(ctx, err)
				}
			}
		case channelWhatsApp:
			switch {
			case s.WhatsApp == nil:
				reason = "WhatsApp belum dikonfigurasi."
			case acc.ContactID() == "":
				reason = reasonWANoContact
			case senderUserID == "":
				reason = reasonNoSender
			default:
				if _, _, err := s.WhatsApp.SendInvoiceWhatsApp(ctx, scope, WhatsAppInput{
					SenderUserID: senderUserID, ContactID: acc.ContactID(), Text: subject + "\n\n" + text,
				}); err != nil {
					reason = s.deliveryReason(ctx, err)
				}
			}
		default:
			reason = "Kanal tidak dikenal."
		}
		if reason == "" {
			sent = true
			continue
		}
		failures = append(failures, channelLabel(channel)+": "+reason)
	}
	errText = strings.Join(failures, "; ")
	if len(channels) == 0 {
		errText = "Tidak ada kanal pengiriman."
	}
	if sent {
		return domain.ArrearsNotifySent, errText
	}
	return domain.ArrearsNotifyFailed, errText
}

func (s *sendService) deliveryReason(ctx context.Context, err error) string {
	if res := s.finish(ctx, domain.Invoice{}, attempt{}, "", err); res.unavailable != "" {
		return res.unavailable
	}
	slog.WarnContext(ctx, "receivable: arrears summary delivery failed", "error", err)
	return reasonSendFailed
}

// NotifyArrears implements ArrearsNotifier: one summary for the recording's invoices, sent by
// the default sender (fallback: the recording user) over the first invoice's channels.
func (s *sendService) NotifyArrears(ctx context.Context, scope coretenant.Scope, rec domain.ArrearsRecording, invoices []domain.Invoice, actorUserID string) (string, string) {
	if len(invoices) == 0 {
		return domain.ArrearsNotifySkipped, "Tidak ada invoice."
	}
	sender := actorUserID
	if s.Settings != nil {
		if settings, err := s.Settings.Get(ctx, scope); err == nil {
			sender = firstNonEmpty(settings.DefaultSenderUserID, actorUserID)
		}
	}
	rows := make([]ArrearsMessageRow, len(invoices))
	total := new(big.Rat)
	for i, inv := range invoices {
		link := ""
		if s.Docs != nil {
			if l, _, err := s.Docs.Link(ctx, scope, inv.ID, sender); err == nil {
				link = l
			} else {
				slog.WarnContext(ctx, "receivable: arrears link unavailable", "invoice_id", inv.ID, "error", err)
			}
		}
		row := ArrearsMessageRow{Number: inv.InvoiceNumber, Amount: docpdf.FormatRupiah(inv.Balance()), Link: link}
		if inv.PeriodStart != nil && inv.PeriodEnd != nil {
			row.Period = periodLabel(*inv.PeriodStart, *inv.PeriodEnd)
		}
		if inv.DueDate != nil {
			row.DueDate = docpdf.FormatDateID(*inv.DueDate)
		}
		rows[i] = row
		if v, ok := new(big.Rat).SetString(inv.Balance()); ok {
			total.Add(total, v)
		}
	}
	requestID := fmt.Sprintf("arrears-%s-%d", rec.ID, time.Now().UnixNano())
	return s.SendArrearsSummary(ctx, scope, invoices[0].Account, invoices[0].Channels, rows,
		docpdf.FormatRupiah(total.FloatString(2)), sender, requestID)
}

var _ ArrearsNotifier = (*sendService)(nil)
