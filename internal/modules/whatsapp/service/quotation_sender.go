package service

import (
	"context"
	"errors"
	"unicode/utf8"

	coretenant "zyad.cloud/internal/core/tenant"
	crmservice "zyad.cloud/internal/modules/crm/service"
	whatsappmodule "zyad.cloud/internal/modules/whatsapp"
	"zyad.cloud/internal/modules/whatsapp/domain"
)

type FeatureGate interface {
	RequireFeature(ctx context.Context, organizationID, featureKey string) error
}

// QuotationSender mengirim penawaran CRM lewat percakapan WhatsApp kontak.
type QuotationSender struct {
	conversations *ConversationService
	gate          FeatureGate
}

func NewQuotationSender(conversations *ConversationService, gate FeatureGate) *QuotationSender {
	return &QuotationSender{conversations: conversations, gate: gate}
}

func channelErr(reason string) error { return &crmservice.ChannelError{Reason: reason} }

func mapStartError(err error) error {
	switch {
	case errors.Is(err, whatsappmodule.ErrEntityPhoneInvalid):
		return channelErr("Nomor WhatsApp kontak tidak valid.")
	case errors.Is(err, whatsappmodule.ErrNoConnectedSession), errors.Is(err, whatsappmodule.ErrSessionNotConnected):
		return channelErr("Belum ada session WhatsApp yang tersambung.")
	case errors.Is(err, whatsappmodule.ErrConversationForbidden):
		return channelErr("Percakapan kontak ini milik sales lain.")
	case errors.Is(err, whatsappmodule.ErrNotConfigured):
		return channelErr("WhatsApp belum dikonfigurasi.")
	case errors.Is(err, whatsappmodule.ErrEntityNotFound):
		return channelErr("Kontak tidak ditemukan.")
	}
	return err
}

func (s *QuotationSender) SendQuotationWhatsApp(ctx context.Context, scope coretenant.Scope, in crmservice.QuotationWhatsAppInput) (string, string, error) {
	if s.gate != nil {
		if err := s.gate.RequireFeature(ctx, scope.OrganizationID(), "whatsapp.enabled"); err != nil {
			return "", "", channelErr("WhatsApp belum aktif untuk organisasi ini.")
		}
	}
	viewer := Viewer{UserID: in.UserID, CanReadAll: in.CanReadAll}
	conv, err := s.conversations.Start(ctx, scope, viewer, StartConversationInput{
		SessionID: in.SessionID, RelatedEntityType: domain.RelatedEntityContact, RelatedEntityID: in.ContactID,
	})
	if err != nil {
		return "", "", mapStartError(err)
	}
	var last domain.Message
	sendText := func(text string) error {
		m, err := s.conversations.Send(ctx, scope, viewer, conv.ID, text)
		if err != nil {
			return mapStartError(err)
		}
		if m.Status == domain.MessageStatusFailed {
			return errors.New("Pesan WhatsApp gagal terkirim: " + m.Error)
		}
		last = m
		return nil
	}
	sendDoc := func(caption string, textAlreadySent bool) error {
		m, err := s.conversations.SendDocument(ctx, scope, viewer, conv.ID, DocumentInput{
			Filename: in.Attachment.Filename, MimeType: in.Attachment.MimeType, Data: in.Attachment.Content, Caption: caption,
		})
		if err != nil {
			return mapStartError(err)
		}
		if m.Status == domain.MessageStatusFailed {
			if textAlreadySent {
				return errors.New("Teks terkirim, dokumen gagal dikirim: " + m.Error)
			}
			return errors.New("Dokumen WhatsApp gagal terkirim: " + m.Error)
		}
		last = m
		return nil
	}

	switch {
	case in.Attachment == nil:
		err = sendText(in.Text)
	case in.Text == "":
		err = sendDoc("", false)
	case utf8.RuneCountInString(in.Text) <= MaxDocumentCaption:
		err = sendDoc(in.Text, false)
	default:
		if err = sendText(in.Text); err == nil {
			err = sendDoc("", true)
		}
	}
	if err != nil {
		return "", conv.PhoneNormalized, err
	}
	return last.WAHAMessageID, conv.PhoneNormalized, nil
}
