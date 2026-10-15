package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
)

// InvoiceSender mengirim invoice receivable lewat percakapan WhatsApp kontak CRM.
// Pengirim adalah pengguna penagihan (PIC/pengirim default), jadi percakapan dibuka
// dengan CanReadAll: kepemilikan percakapan CRM tidak membatasi pengiriman invoice.
type InvoiceSender struct{ sender documentSender }

func NewInvoiceSender(conversations *ConversationService, gate FeatureGate) *InvoiceSender {
	return &InvoiceSender{sender: documentSender{conversations: conversations, gate: gate}}
}

func (s *InvoiceSender) SendInvoiceWhatsApp(ctx context.Context, scope coretenant.Scope, in receivableservice.WhatsAppInput) (string, string, error) {
	msg := documentMessage{UserID: in.SenderUserID, CanReadAll: true, ContactID: in.ContactID, Text: in.Text}
	if in.Attachment != nil {
		msg.Attachment = &docAttachment{Filename: in.Attachment.Filename, MimeType: in.Attachment.MimeType, Content: in.Attachment.Content}
	}
	return s.sender.send(ctx, scope, msg, func(reason string) error { return &receivableservice.ChannelError{Reason: reason} })
}
