package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/realtime"
)

// publishChange tells open SSE streams that a conversation changed. It is a
// no-op without a publisher, and never fails the caller (see realtime.Publisher).
func publishChange(ctx context.Context, publisher realtime.Publisher, scope coretenant.Scope, eventType realtime.EventType, conversation domain.Conversation, messageID string) {
	if publisher == nil {
		return
	}
	publisher.Publish(ctx, scope.OrganizationID(), realtime.Event{
		Type:              eventType,
		ConversationID:    conversation.ID,
		MessageID:         messageID,
		AssigneeUserID:    conversation.AssigneeUserID,
		RelatedEntityType: string(conversation.RelatedEntityType),
		RelatedEntityID:   conversation.RelatedEntityID,
	})
}
