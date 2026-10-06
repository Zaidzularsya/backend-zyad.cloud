package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	crmservice "zyad.cloud/internal/modules/crm/service"
)

// QuotationSender mengirim penawaran CRM lewat percakapan WhatsApp kontak.
type QuotationSender struct{ sender documentSender }

func NewQuotationSender(conversations *ConversationService, gate FeatureGate) *QuotationSender {
	return &QuotationSender{sender: documentSender{conversations: conversations, gate: gate}}
}

func (s *QuotationSender) SendQuotationWhatsApp(ctx context.Context, scope coretenant.Scope, in crmservice.QuotationWhatsAppInput) (string, string, error) {
	msg := documentMessage{UserID: in.UserID, CanReadAll: in.CanReadAll, SessionID: in.SessionID, ContactID: in.ContactID, Text: in.Text}
	if in.Attachment != nil {
		msg.Attachment = &docAttachment{Filename: in.Attachment.Filename, MimeType: in.Attachment.MimeType, Content: in.Attachment.Content}
	}
	return s.sender.send(ctx, scope, msg, func(reason string) error { return &crmservice.ChannelError{Reason: reason} })
}
