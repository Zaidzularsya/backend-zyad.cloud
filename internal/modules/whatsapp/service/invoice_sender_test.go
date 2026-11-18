package service

import (
	"errors"
	"strings"
	"testing"

	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/modules/whatsapp/domain"
)

// Pengirim invoice adalah pengguna penagihan, bukan pemilik percakapan CRM: kontak milik
// "sales-2" tetap bisa dikirimi oleh "finance-1" karena adapter memakai CanReadAll.
func TestInvoiceSenderSendsCaptionedDocumentFromBillingUser(t *testing.T) {
	h := newConversationHarness(t)
	sender := NewInvoiceSender(h.svc, allowGate{})
	id, recipient, err := sender.SendInvoiceWhatsApp(ctxTest, h.scope, receivableservice.WhatsAppInput{
		SenderUserID: "finance-1", ContactID: "contact", Text: "Halo Sari, berikut invoice.",
		Attachment: &receivableservice.Attachment{Filename: "INV-1.pdf", MimeType: "application/pdf", Content: []byte("%PDF")},
	})
	if err != nil || id != "true_file_INV-1.pdf" || recipient != "6281277770000" {
		t.Fatalf("id=%q recipient=%q err=%v", id, recipient, err)
	}
	if len(h.sender.sent) != 0 || len(h.sender.files) != 1 || h.sender.files[0].Caption != "Halo Sari, berikut invoice." {
		t.Fatalf("sent=%d files=%+v", len(h.sender.sent), h.sender.files)
	}
}

func TestInvoiceSenderLongTextGoesSeparately(t *testing.T) {
	h := newConversationHarness(t)
	long := strings.Repeat("x", MaxDocumentCaption+1)
	if _, _, err := NewInvoiceSender(h.svc, allowGate{}).SendInvoiceWhatsApp(ctxTest, h.scope, receivableservice.WhatsAppInput{
		SenderUserID: "finance-1", ContactID: "contact", Text: long,
		Attachment: &receivableservice.Attachment{Filename: "I.pdf", MimeType: "application/pdf", Content: []byte("%PDF")},
	}); err != nil {
		t.Fatal(err)
	}
	if len(h.sender.sent) != 1 || len(h.sender.files) != 1 || h.sender.files[0].Caption != "" {
		t.Fatalf("sent=%d files=%+v", len(h.sender.sent), h.sender.files)
	}
}

func TestInvoiceSenderChannelErrors(t *testing.T) {
	h := newConversationHarness(t)
	in := receivableservice.WhatsAppInput{SenderUserID: "finance-1", ContactID: "contact", Text: "x"}
	if _, _, err := NewInvoiceSender(h.svc, allowGate{err: errors.New("off")}).SendInvoiceWhatsApp(ctxTest, h.scope, in); !errors.Is(err, receivableservice.ErrChannelUnavailable) {
		t.Fatalf("gate err = %v", err)
	}
	h.crm.entities["c-nophone"] = CRMEntity{Type: domain.RelatedEntityContact, ID: "c-nophone", Name: "X", Phone: "-"}
	if _, _, err := NewInvoiceSender(h.svc, allowGate{}).SendInvoiceWhatsApp(ctxTest, h.scope,
		receivableservice.WhatsAppInput{SenderUserID: "finance-1", ContactID: "c-nophone", Text: "x"}); !errors.Is(err, receivableservice.ErrChannelUnavailable) {
		t.Fatalf("phone err = %v", err)
	}
	h.sender.fileErr = errors.New("boom")
	_, _, err := NewInvoiceSender(h.svc, allowGate{}).SendInvoiceWhatsApp(ctxTest, h.scope, receivableservice.WhatsAppInput{
		SenderUserID: "finance-1", ContactID: "contact", Attachment: &receivableservice.Attachment{Filename: "I.pdf", MimeType: "application/pdf", Content: []byte("x")},
	})
	if err == nil || errors.Is(err, receivableservice.ErrChannelUnavailable) {
		t.Fatalf("delivery failure must be a plain error, got %v", err)
	}
}

// WAHA WEBJS kadang gagal mengirim file (500 dari WhatsApp Web). Bila teks memuat
// link invoice (FallbackToText), pelanggan tetap menerima teks + link.
func TestInvoiceSenderFallsBackToTextWhenDocumentFails(t *testing.T) {
	h := newConversationHarness(t)
	h.sender.fileErr = errors.New("waha send file failed (status 500)")
	text := "Halo Sari, berikut invoice.\nLihat & bayar: https://zyad.online/i/abc"
	id, recipient, err := NewInvoiceSender(h.svc, allowGate{}).SendInvoiceWhatsApp(ctxTest, h.scope, receivableservice.WhatsAppInput{
		SenderUserID: "finance-1", ContactID: "contact", Text: text, FallbackToText: true,
		Attachment: &receivableservice.Attachment{Filename: "INV-1.pdf", MimeType: "application/pdf", Content: []byte("%PDF")},
	})
	if err != nil || id != "true_waha_"+text || recipient != "6281277770000" {
		t.Fatalf("id=%q recipient=%q err=%v", id, recipient, err)
	}
	if len(h.sender.sent) != 1 || h.sender.sent[0].Text != text {
		t.Fatalf("sent=%+v", h.sender.sent)
	}
}

func TestInvoiceSenderLongTextFallbackKeepsTextAlreadySent(t *testing.T) {
	h := newConversationHarness(t)
	h.sender.fileErr = errors.New("boom")
	long := strings.Repeat("x", MaxDocumentCaption+1)
	id, _, err := NewInvoiceSender(h.svc, allowGate{}).SendInvoiceWhatsApp(ctxTest, h.scope, receivableservice.WhatsAppInput{
		SenderUserID: "finance-1", ContactID: "contact", Text: long, FallbackToText: true,
		Attachment: &receivableservice.Attachment{Filename: "I.pdf", MimeType: "application/pdf", Content: []byte("%PDF")},
	})
	if err != nil || id != "true_waha_"+long {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if len(h.sender.sent) != 1 {
		t.Fatalf("text must not be sent twice, sent=%d", len(h.sender.sent))
	}
}

func TestInvoiceSenderWithoutFallbackStillFailsOnDocumentError(t *testing.T) {
	h := newConversationHarness(t)
	h.sender.fileErr = errors.New("boom")
	_, _, err := NewInvoiceSender(h.svc, allowGate{}).SendInvoiceWhatsApp(ctxTest, h.scope, receivableservice.WhatsAppInput{
		SenderUserID: "finance-1", ContactID: "contact", Text: "Halo",
		Attachment: &receivableservice.Attachment{Filename: "I.pdf", MimeType: "application/pdf", Content: []byte("%PDF")},
	})
	if err == nil || len(h.sender.sent) != 0 {
		t.Fatalf("err=%v sent=%d", err, len(h.sender.sent))
	}
}
