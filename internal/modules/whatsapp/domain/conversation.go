package domain

import "time"

type RelatedEntityType string

const (
	RelatedEntityLead    RelatedEntityType = "lead"
	RelatedEntityContact RelatedEntityType = "contact"
)

func (t RelatedEntityType) IsValid() bool {
	return t == RelatedEntityLead || t == RelatedEntityContact
}

type ConversationStatus string

const (
	ConversationStatusOpen   ConversationStatus = "open"
	ConversationStatusClosed ConversationStatus = "closed"
)

// Conversation is one WhatsApp chat (session + chat id), optionally linked to
// a CRM lead or contact and assigned to a user (default: the entity owner).
type Conversation struct {
	ID                 string
	OrganizationID     string
	SessionID          string
	ChatID             string
	PhoneNormalized    string
	ContactName        string
	RelatedEntityType  RelatedEntityType
	RelatedEntityID    string
	AssigneeUserID     string
	LastMessageAt      *time.Time
	LastMessagePreview string
	UnreadCount        int
	Status             ConversationStatus
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
