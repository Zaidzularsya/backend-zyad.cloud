package service

import (
	"context"
	"errors"

	coretenant "zyad.cloud/internal/core/tenant"
)

// Antarmuka kanal: diimplementasikan adapter di modul mailbox & whatsapp
// (receivable tidak mengimpor keduanya).

type Attachment struct {
	Filename, MimeType string
	Content            []byte
}

// ChannelError: kanal tidak bisa dipakai (validasi/konfigurasi), bukan kegagalan kirim.
// Reason aman ditampilkan ke pengguna.
type ChannelError struct{ Reason string }

func (e *ChannelError) Error() string { return e.Reason }

var ErrChannelUnavailable = errors.New("channel unavailable")

func (e *ChannelError) Is(target error) bool { return target == ErrChannelUnavailable }

type EmailInput struct {
	SenderUserID, To, Subject, BodyHTML, ContactID, ClientRequestID string
	Attachment                                                      *Attachment
}

// WhatsAppInput dikirim dengan CanReadAll=true oleh adapter: pengirim invoice adalah
// pengguna penagihan, bukan pemilik percakapan CRM.
type WhatsAppInput struct {
	SenderUserID, ContactID, Text string
	Attachment                    *Attachment
}

type EmailSender interface {
	SendInvoiceEmail(ctx context.Context, scope coretenant.Scope, in EmailInput) (externalID string, err error)
}

type WhatsAppSender interface {
	SendInvoiceWhatsApp(ctx context.Context, scope coretenant.Scope, in WhatsAppInput) (externalID, recipient string, err error)
}
