package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	crmservice "zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/modules/mailbox/domain"
)

type stubMailboxes struct{ list []domain.Mailbox }

func (s stubMailboxes) List(context.Context, coretenant.Scope, string) ([]domain.Mailbox, error) {
	return s.list, nil
}

type stubMessages struct {
	got SendInput
	msg domain.Message
	err error
}

func (s *stubMessages) Send(_ context.Context, _ coretenant.Scope, _ string, mailboxID string, in SendInput) (domain.Message, error) {
	s.got = in
	s.msg.MailboxID = mailboxID
	return s.msg, s.err
}

func TestQuotationMailerPicksActiveMailboxAndAttachesPDF(t *testing.T) {
	msgs := &stubMessages{msg: domain.Message{ID: "m1", Status: domain.MessageStatusQueued}}
	mailer := NewQuotationMailer(stubMailboxes{list: []domain.Mailbox{
		{ID: "mb-off", Status: domain.MailboxStatusDisabled}, {ID: "mb-1", Status: domain.MailboxStatusActive},
	}}, msgs)
	id, err := mailer.SendQuotationEmail(context.Background(), coretenant.Scope{}, crmservice.QuotationEmailInput{
		UserID: "u1", ContactID: "c1", ClientRequestID: "req-1", To: "budi@example.com", Subject: "Penawaran", BodyHTML: "<p>x</p>",
		Attachment: &crmservice.QuotationAttachment{Filename: "Q.pdf", MimeType: "application/pdf", Content: []byte("%PDF")},
	})
	if err != nil || id != "m1" || msgs.msg.MailboxID != "mb-1" {
		t.Fatalf("id=%q mailbox=%q err=%v", id, msgs.msg.MailboxID, err)
	}
	if msgs.got.RelatedEntityType != domain.RelatedEntityContact || msgs.got.RelatedEntityID != "c1" || msgs.got.ClientRequestID != "req-1" ||
		len(msgs.got.Files) != 1 || msgs.got.Files[0].Filename != "Q.pdf" || msgs.got.To[0] != "budi@example.com" {
		t.Fatalf("input = %+v", msgs.got)
	}
}

func TestQuotationMailerWithoutMailboxIsChannelError(t *testing.T) {
	mailer := NewQuotationMailer(stubMailboxes{}, &stubMessages{})
	if _, err := mailer.SendQuotationEmail(context.Background(), coretenant.Scope{}, crmservice.QuotationEmailInput{UserID: "u1", To: "a@b.c"}); !errors.Is(err, crmservice.ErrChannelUnavailable) {
		t.Fatalf("err = %v", err)
	}
}
