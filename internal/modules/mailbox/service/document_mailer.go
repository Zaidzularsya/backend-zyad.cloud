package service

import (
	"bytes"
	"context"
	"errors"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	mailboxmodule "zyad.cloud/internal/modules/mailbox"
	"zyad.cloud/internal/modules/mailbox/domain"
)

type MailboxLister interface {
	List(ctx context.Context, scope coretenant.Scope, userID string) ([]domain.Mailbox, error)
}

type MessageSenderService interface {
	Send(ctx context.Context, scope coretenant.Scope, userID, mailboxID string, input SendInput) (domain.Message, error)
}

type docAttachment struct {
	Filename string
	Content  []byte
}

// documentEmail adalah email dokumen bisnis (penawaran, invoice). ContactID kosong = tanpa
// relasi CRM (tenant tanpa CRM / account manual).
type documentEmail struct {
	UserID, MailboxID, To, Subject, BodyHTML, ContactID, ClientRequestID string
	Attachment                                                           *docAttachment
}

// documentMailer adalah logika bersama QuotationMailer dan InvoiceMailer: memilih mailbox,
// mengirim, dan memetakan galat. unavailable membungkus alasan ke tipe ChannelError milik pemanggil
// (mailbox tidak mengikat diri ke satu modul dokumen).
type documentMailer struct {
	mailboxes MailboxLister
	messages  MessageSenderService
}

func (d documentMailer) send(ctx context.Context, scope coretenant.Scope, in documentEmail, unavailable func(reason string) error) (string, error) {
	list, err := d.mailboxes.List(ctx, scope, in.UserID)
	if err != nil {
		return "", err
	}
	mailboxID := ""
	for _, mb := range list {
		if in.MailboxID != "" && mb.ID == in.MailboxID {
			mailboxID = mb.ID
			break
		}
		if in.MailboxID == "" && mb.Status == domain.MailboxStatusActive {
			mailboxID = mb.ID
			break
		}
	}
	if mailboxID == "" {
		return "", unavailable("Belum ada mailbox aktif. Hubungkan email di menu Email.")
	}
	input := SendInput{
		ClientRequestID: in.ClientRequestID, To: []string{in.To}, Subject: in.Subject, BodyHTML: in.BodyHTML,
	}
	if in.ContactID != "" {
		input.RelatedEntityType, input.RelatedEntityID = domain.RelatedEntityContact, in.ContactID
	}
	if in.Attachment != nil {
		input.Files = []FileInput{{Filename: in.Attachment.Filename, Size: int64(len(in.Attachment.Content)), Content: bytes.NewReader(in.Attachment.Content)}}
	}
	msg, err := d.messages.Send(ctx, scope, in.UserID, mailboxID, input)
	switch {
	case errors.Is(err, mailboxmodule.ErrMailboxInactive):
		return "", unavailable("Mailbox tidak aktif.")
	case errors.Is(err, mailboxmodule.ErrAttachmentsTooLarge):
		return "", unavailable("PDF terlalu besar untuk email.")
	case isValidationError(err):
		return "", unavailable("Alamat email penerima tidak valid.")
	case err != nil:
		return "", err
	}
	if msg.Status == domain.MessageStatusFailed {
		return "", errors.New("Email gagal dikirim.")
	}
	return msg.ID, nil
}

func isValidationError(err error) bool {
	var appErr *coreerrors.AppError
	return errors.As(err, &appErr) && appErr.Code == "VALIDATION_ERROR"
}
