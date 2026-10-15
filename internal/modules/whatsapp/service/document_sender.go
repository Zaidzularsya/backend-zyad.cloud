package service

import (
	"context"
	"errors"
	"unicode/utf8"

	coretenant "zyad.cloud/internal/core/tenant"
	whatsappmodule "zyad.cloud/internal/modules/whatsapp"
	"zyad.cloud/internal/modules/whatsapp/domain"
)

type FeatureGate interface {
	RequireFeature(ctx context.Context, organizationID, featureKey string) error
}

type docAttachment struct {
	Filename, MimeType string
	Content            []byte
}

// documentMessage adalah pesan dokumen bisnis (penawaran, invoice) ke WhatsApp kontak CRM.
type documentMessage struct {
	UserID     string
	CanReadAll bool
	SessionID  string
	ContactID  string
	Text       string // "" = tanpa teks
	Attachment *docAttachment
}

// documentSender adalah logika bersama QuotationSender dan InvoiceSender. unavailable
// membungkus alasan ke tipe ChannelError milik pemanggil.
type documentSender struct {
	conversations *ConversationService
	gate          FeatureGate
}

func mapStartError(err error, unavailable func(string) error) error {
	switch {
	case errors.Is(err, whatsappmodule.ErrEntityPhoneInvalid):
		return unavailable("Nomor WhatsApp kontak tidak valid.")
	case errors.Is(err, whatsappmodule.ErrNoConnectedSession), errors.Is(err, whatsappmodule.ErrSessionNotConnected):
		return unavailable("Belum ada session WhatsApp yang tersambung.")
	case errors.Is(err, whatsappmodule.ErrConversationForbidden):
		return unavailable("Percakapan kontak ini milik sales lain.")
	case errors.Is(err, whatsappmodule.ErrNotConfigured):
		return unavailable("WhatsApp belum dikonfigurasi.")
	case errors.Is(err, whatsappmodule.ErrEntityNotFound):
		return unavailable("Kontak tidak ditemukan.")
	}
	return err
}

// send mengembalikan id pesan WAHA terakhir dan nomor tujuan ternormalisasi.
func (d documentSender) send(ctx context.Context, scope coretenant.Scope, in documentMessage, unavailable func(string) error) (string, string, error) {
	if d.gate != nil {
		if err := d.gate.RequireFeature(ctx, scope.OrganizationID(), "whatsapp.enabled"); err != nil {
			return "", "", unavailable("WhatsApp belum aktif untuk organisasi ini.")
		}
	}
	viewer := Viewer{UserID: in.UserID, CanReadAll: in.CanReadAll}
	conv, err := d.conversations.Start(ctx, scope, viewer, StartConversationInput{
		SessionID: in.SessionID, RelatedEntityType: domain.RelatedEntityContact, RelatedEntityID: in.ContactID,
	})
	if err != nil {
		return "", "", mapStartError(err, unavailable)
	}
	var last domain.Message
	sendText := func(text string) error {
		m, err := d.conversations.Send(ctx, scope, viewer, conv.ID, text)
		if err != nil {
			return mapStartError(err, unavailable)
		}
		if m.Status == domain.MessageStatusFailed {
			return errors.New("Pesan WhatsApp gagal terkirim: " + m.Error)
		}
		last = m
		return nil
	}
	sendDoc := func(caption string, textAlreadySent bool) error {
		m, err := d.conversations.SendDocument(ctx, scope, viewer, conv.ID, DocumentInput{
			Filename: in.Attachment.Filename, MimeType: in.Attachment.MimeType, Data: in.Attachment.Content, Caption: caption,
		})
		if err != nil {
			return mapStartError(err, unavailable)
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
