package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
)

// InvoiceMailer mengirim invoice receivable dari mailbox milik pengirim yang dipilih.
type InvoiceMailer struct{ mailer documentMailer }

func NewInvoiceMailer(mailboxes MailboxLister, messages MessageSenderService) *InvoiceMailer {
	return &InvoiceMailer{mailer: documentMailer{mailboxes: mailboxes, messages: messages}}
}

func (m *InvoiceMailer) SendInvoiceEmail(ctx context.Context, scope coretenant.Scope, in receivableservice.EmailInput) (string, error) {
	email := documentEmail{
		UserID: in.SenderUserID, To: in.To, Subject: in.Subject, BodyHTML: in.BodyHTML,
		ContactID: in.ContactID, ClientRequestID: in.ClientRequestID,
	}
	if in.Attachment != nil {
		email.Attachment = &docAttachment{Filename: in.Attachment.Filename, Content: in.Attachment.Content}
	}
	return m.mailer.send(ctx, scope, email, func(reason string) error { return &receivableservice.ChannelError{Reason: reason} })
}
