package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	crmservice "zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/modules/whatsapp/domain"
)

type allowGate struct{ err error }

func (g allowGate) RequireFeature(context.Context, string, string) error { return g.err }

func TestQuotationSenderSendsCaptionedDocument(t *testing.T) {
	h := newConversationHarness(t)
	sender := NewQuotationSender(h.svc, allowGate{})
	id, recipient, err := sender.SendQuotationWhatsApp(ctxTest, h.scope, crmservice.QuotationWhatsAppInput{
		UserID: "sales-2", ContactID: "contact", Text: "Halo Sari, berikut penawaran.",
		Attachment: &crmservice.QuotationAttachment{Filename: "QUO-1.pdf", MimeType: "application/pdf", Content: []byte("%PDF")},
	})
	if err != nil || id != "true_file_QUO-1.pdf" || recipient != "6281277770000" {
		t.Fatalf("id=%q recipient=%q err=%v", id, recipient, err)
	}
	if len(h.sender.sent) != 0 || len(h.sender.files) != 1 || h.sender.files[0].Caption != "Halo Sari, berikut penawaran." {
		t.Fatalf("sent=%d files=%+v", len(h.sender.sent), h.sender.files)
	}
}

func TestQuotationSenderLongTextGoesSeparately(t *testing.T) {
	h := newConversationHarness(t)
	sender := NewQuotationSender(h.svc, allowGate{})
	long := strings.Repeat("x", MaxDocumentCaption+1)
	if _, _, err := sender.SendQuotationWhatsApp(ctxTest, h.scope, crmservice.QuotationWhatsAppInput{
		UserID: "sales-2", ContactID: "contact", Text: long,
		Attachment: &crmservice.QuotationAttachment{Filename: "Q.pdf", MimeType: "application/pdf", Content: []byte("%PDF")},
	}); err != nil {
		t.Fatal(err)
	}
	if len(h.sender.sent) != 1 || len(h.sender.files) != 1 || h.sender.files[0].Caption != "" {
		t.Fatalf("sent=%d files=%+v", len(h.sender.sent), h.sender.files)
	}
}

func TestQuotationSenderChannelErrors(t *testing.T) {
	h := newConversationHarness(t)
	if _, _, err := NewQuotationSender(h.svc, allowGate{err: errors.New("off")}).SendQuotationWhatsApp(ctxTest, h.scope,
		crmservice.QuotationWhatsAppInput{UserID: "sales-2", ContactID: "contact", Text: "x"}); !errors.Is(err, crmservice.ErrChannelUnavailable) {
		t.Fatalf("gate err = %v", err)
	}
	h.crm.entities["c-nophone"] = CRMEntity{Type: domain.RelatedEntityContact, ID: "c-nophone", Name: "X", Phone: "-"}
	if _, _, err := NewQuotationSender(h.svc, allowGate{}).SendQuotationWhatsApp(ctxTest, h.scope,
		crmservice.QuotationWhatsAppInput{UserID: "sales-2", CanReadAll: true, ContactID: "c-nophone", Text: "x"}); !errors.Is(err, crmservice.ErrChannelUnavailable) {
		t.Fatalf("phone err = %v", err)
	}
	h.sender.fileErr = errors.New("boom")
	_, _, err := NewQuotationSender(h.svc, allowGate{}).SendQuotationWhatsApp(ctxTest, h.scope, crmservice.QuotationWhatsAppInput{
		UserID: "sales-2", ContactID: "contact", Attachment: &crmservice.QuotationAttachment{Filename: "Q.pdf", MimeType: "application/pdf", Content: []byte("x")},
	})
	if err == nil || errors.Is(err, crmservice.ErrChannelUnavailable) {
		t.Fatalf("delivery failure must be a plain error, got %v", err)
	}
}
