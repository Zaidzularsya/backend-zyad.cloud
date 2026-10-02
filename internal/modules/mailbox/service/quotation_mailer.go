package service

import (
	"bytes"
	"context"
	"errors"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	crmservice "zyad.cloud/internal/modules/crm/service"
	mailboxmodule "zyad.cloud/internal/modules/mailbox"
	"zyad.cloud/internal/modules/mailbox/domain"
)

type MailboxLister interface {
	List(ctx context.Context, scope coretenant.Scope, userID string) ([]domain.Mailbox, error)
}

type MessageSenderService interface {
	Send(ctx context.Context, scope coretenant.Scope, userID, mailboxID string, input SendInput) (domain.Message, error)
}

// QuotationMailer mengirim penawaran CRM dari mailbox milik pengguna.
type QuotationMailer struct {
	mailboxes MailboxLister
	messages  MessageSenderService
}

func NewQuotationMailer(mailboxes MailboxLister, messages MessageSenderService) *QuotationMailer {
	return &QuotationMailer{mailboxes: mailboxes, messages: messages}
}

func (m *QuotationMailer) SendQuotationEmail(ctx context.Context, scope coretenant.Scope, in crmservice.QuotationEmailInput) (string, error) {
	list, err := m.mailboxes.List(ctx, scope, in.UserID)
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
		return "", &crmservice.ChannelError{Reason: "Belum ada mailbox aktif. Hubungkan email di menu Email."}
	}
	input := SendInput{
		ClientRequestID: in.ClientRequestID, To: []string{in.To}, Subject: in.Subject, BodyHTML: in.BodyHTML,
		RelatedEntityType: domain.RelatedEntityContact, RelatedEntityID: in.ContactID,
	}
	if in.Attachment != nil {
		input.Files = []FileInput{{Filename: in.Attachment.Filename, Size: int64(len(in.Attachment.Content)), Content: bytes.NewReader(in.Attachment.Content)}}
	}
	msg, err := m.messages.Send(ctx, scope, in.UserID, mailboxID, input)
	switch {
	case errors.Is(err, mailboxmodule.ErrMailboxInactive):
		return "", &crmservice.ChannelError{Reason: "Mailbox tidak aktif."}
	case errors.Is(err, mailboxmodule.ErrAttachmentsTooLarge):
		return "", &crmservice.ChannelError{Reason: "PDF terlalu besar untuk email."}
	case isValidationError(err):
		return "", &crmservice.ChannelError{Reason: "Alamat email penerima tidak valid."}
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
