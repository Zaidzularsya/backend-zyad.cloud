package service

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/quotationpdf"
	"zyad.cloud/internal/modules/crm/repository"
)

var (
	ErrQuotationNotSendable = errors.New("only draft or sent quotations can be sent")
	ErrInvalidSendInput     = errors.New("channel must be email or whatsapp, mode text|pdf|text_pdf, client_request_id required (max 100)")
)

func WithQuotationChannels(d QuotationChannelDeps) QuotationServiceOption {
	return func(s *quotationService) { s.channels = &d }
}

func (s *quotationService) documentFor(ctx context.Context, scope coretenant.Scope, q domain.Quotation) (quotationpdf.Document, error) {
	final := q
	if final.Status == domain.QuotationStatusDraft {
		final.Status = domain.QuotationStatusSent
		now := s.now()
		final.SentAt = &now
	}
	return s.buildDocument(ctx, scope, final)
}

func (s *quotationService) Summary(ctx context.Context, scope coretenant.Scope, id, opening string) (quotationpdf.Summary, error) {
	if s.docs == nil {
		return quotationpdf.Summary{}, ErrQuotationDocumentsUnavailable
	}
	q, err := s.Get(ctx, scope, id)
	if err != nil {
		return quotationpdf.Summary{}, err
	}
	doc, err := s.documentFor(ctx, scope, q)
	if err != nil {
		return quotationpdf.Summary{}, err
	}
	return quotationpdf.BuildSummary(doc, opening), nil
}

func (s *quotationService) ListSends(ctx context.Context, scope coretenant.Scope, id string) ([]domain.QuotationSend, error) {
	if _, err := s.Get(ctx, scope, id); err != nil {
		return nil, err
	}
	if s.channels == nil || s.channels.Sends == nil {
		return []domain.QuotationSend{}, nil
	}
	return s.channels.Sends.ListByQuotation(ctx, scope, id)
}

func (s *quotationService) SendVia(ctx context.Context, scope coretenant.Scope, id string, in SendQuotationInput) (SendQuotationResult, error) {
	if (in.Channel != domain.QuotationChannelEmail && in.Channel != domain.QuotationChannelWhatsApp) ||
		(!in.Mode.NeedsText() && !in.Mode.NeedsPDF()) || in.ClientRequestID == "" || len(in.ClientRequestID) > 100 {
		return SendQuotationResult{}, ErrInvalidSendInput
	}
	if s.docs == nil || s.channels == nil || s.channels.Sends == nil {
		return SendQuotationResult{}, &ChannelError{Reason: "Kanal belum dikonfigurasi."}
	}
	q, err := s.Get(ctx, scope, id)
	if err != nil {
		return SendQuotationResult{}, err
	}
	if q.Status != domain.QuotationStatusDraft && q.Status != domain.QuotationStatusSent {
		return SendQuotationResult{}, ErrQuotationNotSendable
	}
	if existing, err := s.channels.Sends.FindByClientRequest(ctx, scope, q.ID, in.ClientRequestID); err == nil {
		return SendQuotationResult{Quotation: q, Send: existing}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return SendQuotationResult{}, err
	}
	if q.ContactID == nil {
		return SendQuotationResult{}, &ChannelError{Reason: "Quotation belum punya kontak."}
	}

	recipient := ""
	if in.Channel == domain.QuotationChannelEmail {
		if s.channels.Email == nil {
			return SendQuotationResult{}, &ChannelError{Reason: "Email belum dikonfigurasi."}
		}
		recipient = strings.TrimSpace(in.Recipient)
		if recipient == "" && s.docs.Contacts != nil {
			if c, err := s.docs.Contacts.FindByID(ctx, scope, *q.ContactID); err == nil {
				recipient = c.Email
			}
		}
		if recipient == "" {
			return SendQuotationResult{}, &ChannelError{Reason: "Kontak belum punya email."}
		}
		if _, err := mail.ParseAddress(recipient); err != nil {
			return SendQuotationResult{}, &ChannelError{Reason: "Alamat email penerima tidak valid."}
		}
	} else if s.channels.WhatsApp == nil {
		return SendQuotationResult{}, &ChannelError{Reason: "WhatsApp belum dikonfigurasi."}
	}

	doc, err := s.documentFor(ctx, scope, q)
	if err != nil {
		return SendQuotationResult{}, err
	}
	summary := quotationpdf.BuildSummary(doc, in.Message)
	var attachment *QuotationAttachment
	pdfAssetID := ""
	if in.Mode.NeedsPDF() {
		pdf, err := s.ensureSnapshot(ctx, scope, q, in.UserID)
		if err != nil {
			return SendQuotationResult{}, err
		}
		attachment = &QuotationAttachment{Filename: pdf.Filename, MimeType: "application/pdf", Content: pdf.Content}
		if fresh, err := s.repo.FindByID(ctx, scope, q.ID); err == nil && fresh.PDFAssetID != nil {
			pdfAssetID = *fresh.PDFAssetID
		}
	}

	var externalID string
	var sendErr error
	activityType := domain.ActivityTypeEmail
	label := "Email"
	if in.Channel == domain.QuotationChannelEmail {
		body := summary.HTML
		if !in.Mode.NeedsText() {
			body = "<p>" + htmlEscape(quotationpdf.DefaultOpening(doc)) + " Detail terlampir.</p>"
			if strings.TrimSpace(in.Message) != "" {
				body = "<p>" + htmlEscape(in.Message) + "</p>"
			}
		}
		externalID, sendErr = s.channels.Email.SendQuotationEmail(ctx, scope, QuotationEmailInput{
			UserID: in.UserID, MailboxID: in.MailboxID, ContactID: *q.ContactID, ClientRequestID: in.ClientRequestID,
			To: recipient, Subject: summary.Subject, BodyHTML: body, Attachment: attachment,
		})
	} else {
		activityType, label = domain.ActivityTypeWhatsApp, "WhatsApp"
		text := ""
		if in.Mode.NeedsText() {
			text = summary.Text
		}
		externalID, recipient, sendErr = s.channels.WhatsApp.SendQuotationWhatsApp(ctx, scope, QuotationWhatsAppInput{
			UserID: in.UserID, CanReadAll: in.CanReadAllWhatsApp, SessionID: in.SessionID, ContactID: *q.ContactID,
			Text: text, Attachment: attachment,
		})
	}
	if errors.Is(sendErr, ErrChannelUnavailable) {
		return SendQuotationResult{}, sendErr
	}

	status, errText := domain.QuotationSendSent, ""
	if sendErr != nil {
		status, errText = domain.QuotationSendFailed, sendErr.Error()
	}
	record, err := s.channels.Sends.Record(ctx, scope, repository.RecordQuotationSendParams{
		QuotationID: q.ID, Channel: in.Channel, Mode: in.Mode, Recipient: recipient, PDFAssetID: pdfAssetID,
		Status: status, Error: errText, ExternalMessageID: externalID, ClientRequestID: in.ClientRequestID, SentBy: in.UserID,
	})
	if err != nil {
		return SendQuotationResult{}, err
	}
	if sendErr != nil {
		return SendQuotationResult{Quotation: q, Send: record}, nil
	}
	if q.Status == domain.QuotationStatusDraft {
		if sent, err := s.repo.Send(ctx, scope, q.ID, in.UserID); err == nil {
			q = sent
		} else if fresh, ferr := s.repo.FindByID(ctx, scope, q.ID); ferr == nil {
			q = fresh // dikirim bersamaan dari tab lain
		}
	}
	s.recordDealActivity(ctx, scope, q, activityType, "Penawaran "+q.QuotationNumber+" dikirim via "+label+" ke "+recipient, in.UserID)
	return SendQuotationResult{Quotation: q, Send: record}, nil
}

func htmlEscape(v string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;").Replace(v)
}
