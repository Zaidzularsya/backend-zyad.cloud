package domain

import "time"

type MessageDirection string

const (
	MessageDirectionIn  MessageDirection = "in"
	MessageDirectionOut MessageDirection = "out"
)

type MessageStatus string

const (
	MessageStatusPending   MessageStatus = "pending"
	MessageStatusSent      MessageStatus = "sent"
	MessageStatusDelivered MessageStatus = "delivered"
	MessageStatusRead      MessageStatus = "read"
	MessageStatusFailed    MessageStatus = "failed"
)

var messageStatusRank = map[MessageStatus]int{
	MessageStatusPending:   0,
	MessageStatusSent:      1,
	MessageStatusDelivered: 2,
	MessageStatusRead:      3,
}

func (s MessageStatus) IsValid() bool {
	_, ok := messageStatusRank[s]
	return ok || s == MessageStatusFailed
}

// CanTransitionTo reports whether a status update should be applied. WAHA
// acks can arrive out of order, so delivery progress never moves backwards
// (read -> delivered is ignored). failed is only reachable before the message
// was confirmed sent, and a failed message can be retried back to pending.
func (s MessageStatus) CanTransitionTo(next MessageStatus) bool {
	if !next.IsValid() || s == next {
		return false
	}
	if s == MessageStatusFailed {
		return next == MessageStatusPending
	}
	if next == MessageStatusFailed {
		return s == MessageStatusPending
	}
	return messageStatusRank[next] > messageStatusRank[s]
}

// MessageStatusFromAck maps a WAHA ack value (-1..4) to a message status.
func MessageStatusFromAck(ack int) (MessageStatus, bool) {
	switch ack {
	case -1:
		return MessageStatusFailed, true
	case 0:
		return MessageStatusPending, true
	case 1:
		return MessageStatusSent, true
	case 2:
		return MessageStatusDelivered, true
	case 3, 4:
		return MessageStatusRead, true
	default:
		return "", false
	}
}

type Message struct {
	ID             string
	OrganizationID string
	ConversationID string
	WAHAMessageID  string
	Direction      MessageDirection
	Body           string
	Status         MessageStatus
	Error          string
	SentByUserID   string
	SentAt         time.Time
	Raw            map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
