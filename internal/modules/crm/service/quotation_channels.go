package service

import (
	"context"
	"errors"

	coretenant "zyad.cloud/internal/core/tenant"
)

// Kanal kirim penawaran diimplementasikan adapter di modul mailbox & whatsapp
// (mereka sudah bergantung ke CRM, jadi CRM tidak boleh mengimpor mereka).

type QuotationAttachment struct {
	Filename string
	MimeType string
	Content  []byte
}

// ChannelError: kanal tidak bisa dipakai (validasi), bukan kegagalan kirim.
type ChannelError struct{ Reason string }

func (e *ChannelError) Error() string { return e.Reason }

var ErrChannelUnavailable = errors.New("channel unavailable")

func (e *ChannelError) Is(target error) bool { return target == ErrChannelUnavailable }

type QuotationEmailInput struct {
	UserID          string
	MailboxID       string
	ContactID       string
	ClientRequestID string
	To              string
	Subject         string
	BodyHTML        string
	Attachment      *QuotationAttachment
}

type QuotationEmailSender interface {
	SendQuotationEmail(ctx context.Context, scope coretenant.Scope, in QuotationEmailInput) (externalID string, err error)
}

type QuotationWhatsAppInput struct {
	UserID     string
	CanReadAll bool
	SessionID  string
	ContactID  string
	Text       string // "" = tanpa teks
	Attachment *QuotationAttachment
}

type QuotationWhatsAppSender interface {
	SendQuotationWhatsApp(ctx context.Context, scope coretenant.Scope, in QuotationWhatsAppInput) (externalID string, recipient string, err error)
}
