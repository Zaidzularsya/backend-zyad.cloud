package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	crmservice "zyad.cloud/internal/modules/crm/service"
)

// QuotationMailer mengirim penawaran CRM dari mailbox milik pengguna.
type QuotationMailer struct{ mailer documentMailer }

func NewQuotationMailer(mailboxes MailboxLister, messages MessageSenderService) *QuotationMailer {
	return &QuotationMailer{mailer: documentMailer{mailboxes: mailboxes, messages: messages}}
}

func (m *QuotationMailer) SendQuotationEmail(ctx context.Context, scope coretenant.Scope, in crmservice.QuotationEmailInput) (string, error) {
	email := documentEmail{
		UserID: in.UserID, MailboxID: in.MailboxID, To: in.To, Subject: in.Subject, BodyHTML: in.BodyHTML,
		ContactID: in.ContactID, ClientRequestID: in.ClientRequestID,
	}
	if in.Attachment != nil {
		email.Attachment = &docAttachment{Filename: in.Attachment.Filename, Content: in.Attachment.Content}
	}
	return m.mailer.send(ctx, scope, email, func(reason string) error { return &crmservice.ChannelError{Reason: reason} })
}
