package domain

import "time"

type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

type MessageStatus string

const (
	MessageStatusQueued   MessageStatus = "queued"
	MessageStatusSent     MessageStatus = "sent"
	MessageStatusFailed   MessageStatus = "failed"
	MessageStatusReceived MessageStatus = "received"
)

type RelatedEntityType string

const (
	RelatedEntityLead    RelatedEntityType = "lead"
	RelatedEntityContact RelatedEntityType = "contact"
)

func (t RelatedEntityType) IsValid() bool {
	return t == RelatedEntityLead || t == RelatedEntityContact
}

type Message struct {
	ID                string
	MailboxID         string
	Direction         Direction
	Status            MessageStatus
	MessageID         string
	InReplyTo         string
	References        string
	FromAddress       string
	FromName          string
	To                []string
	Cc                []string
	Bcc               []string
	Subject           string
	Snippet           string
	BodyHTML          string
	BodyText          string
	Error             string
	RelatedEntityType RelatedEntityType
	RelatedEntityID   string
	SentAt            *time.Time
	CreatedBy         string
	CreatedAt         time.Time
	Attachments       []Attachment
}

// Attachment reads filename/mime/size from asset_objects.
type Attachment struct {
	ID            string
	MessageID     string
	AssetObjectID string
	Filename      string
	MimeType      string
	SizeBytes     int64
}
