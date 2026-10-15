package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
)

// LeadConversionRelinker moves a lead's WhatsApp conversations to the contact
// it was converted into, so the chat history follows the person to the
// contact page. CRM calls it through crm/service.LeadConvertedHook.
type LeadConversionRelinker struct {
	conversations repository.ConversationRepository
}

func NewLeadConversionRelinker(conversations repository.ConversationRepository) *LeadConversionRelinker {
	return &LeadConversionRelinker{conversations: conversations}
}

func (r *LeadConversionRelinker) LeadConverted(ctx context.Context, scope coretenant.Scope, leadID, contactID string) error {
	_, err := r.conversations.RelinkEntity(ctx, scope, domain.RelatedEntityLead, leadID, domain.RelatedEntityContact, contactID)
	return err
}
