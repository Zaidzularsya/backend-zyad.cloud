package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mailbox/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
)

func TestInvoiceMailerPicksActiveMailboxAndAttachesPDF(t *testing.T) {
	msgs := &stubMessages{msg: domain.Message{ID: "m1", Status: domain.MessageStatusQueued}}
	mailer := NewInvoiceMailer(stubMailboxes{list: []domain.Mailbox{
		{ID: "mb-off", Status: domain.MailboxStatusDisabled}, {ID: "mb-1", Status: domain.MailboxStatusActive},
	}}, msgs)
	id, err := mailer.SendInvoiceEmail(context.Background(), coretenant.Scope{}, receivableservice.EmailInput{
		SenderUserID: "u1", ContactID: "c1", ClientRequestID: "req-1", To: "budi@example.com", Subject: "Invoice INV-1", BodyHTML: "<p>x</p>",
		Attachment: &receivableservice.Attachment{Filename: "INV-1.pdf", MimeType: "application/pdf", Content: []byte("%PDF")},
	})
	if err != nil || id != "m1" || msgs.msg.MailboxID != "mb-1" {
		t.Fatalf("id=%q mailbox=%q err=%v", id, msgs.msg.MailboxID, err)
	}
	if msgs.got.RelatedEntityType != domain.RelatedEntityContact || msgs.got.RelatedEntityID != "c1" || msgs.got.ClientRequestID != "req-1" ||
		len(msgs.got.Files) != 1 || msgs.got.Files[0].Filename != "INV-1.pdf" || msgs.got.To[0] != "budi@example.com" {
		t.Fatalf("input = %+v", msgs.got)
	}
}

// Tenant tanpa CRM / account manual: email tetap terkirim tanpa relasi ke kontak.
func TestInvoiceMailerWithoutContactSendsWithoutRelation(t *testing.T) {
	msgs := &stubMessages{msg: domain.Message{ID: "m2", Status: domain.MessageStatusQueued}}
	mailer := NewInvoiceMailer(stubMailboxes{list: []domain.Mailbox{{ID: "mb-1", Status: domain.MailboxStatusActive}}}, msgs)
	if _, err := mailer.SendInvoiceEmail(context.Background(), coretenant.Scope{}, receivableservice.EmailInput{
		SenderUserID: "u1", To: "budi@example.com", Subject: "Invoice", BodyHTML: "x",
	}); err != nil {
		t.Fatal(err)
	}
	if msgs.got.RelatedEntityType != "" || msgs.got.RelatedEntityID != "" || len(msgs.got.Files) != 0 {
		t.Fatalf("no relation / no files expected: %+v", msgs.got)
	}
}

func TestInvoiceMailerChannelErrors(t *testing.T) {
	in := receivableservice.EmailInput{SenderUserID: "u1", To: "a@b.c"}
	if _, err := NewInvoiceMailer(stubMailboxes{}, &stubMessages{}).SendInvoiceEmail(context.Background(), coretenant.Scope{}, in); !errors.Is(err, receivableservice.ErrChannelUnavailable) {
		t.Fatalf("no mailbox err = %v", err)
	}
	var ce *receivableservice.ChannelError
	if _, err := NewInvoiceMailer(stubMailboxes{}, &stubMessages{}).SendInvoiceEmail(context.Background(), coretenant.Scope{}, in); !errors.As(err, &ce) || ce.Reason == "" {
		t.Fatalf("must be the receivable ChannelError with a reason, got %v", err)
	}
	// delivery failure stays a plain error
	failed := NewInvoiceMailer(stubMailboxes{list: []domain.Mailbox{{ID: "mb-1", Status: domain.MailboxStatusActive}}},
		&stubMessages{msg: domain.Message{Status: domain.MessageStatusFailed}})
	if _, err := failed.SendInvoiceEmail(context.Background(), coretenant.Scope{}, in); err == nil || errors.Is(err, receivableservice.ErrChannelUnavailable) {
		t.Fatalf("failed delivery must be a plain error, got %v", err)
	}
}
