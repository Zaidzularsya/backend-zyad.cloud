package domain

import "time"

// WAHA webhook event types the module processes; others are stored and
// marked processed without action.
const (
	EventSessionStatus = "session.status"
	EventMessage       = "message"
	EventMessageAny    = "message.any"
	EventMessageAck    = "message.ack"
)

// WebhookEvent is a raw WAHA webhook kept for idempotency and retries.
type WebhookEvent struct {
	ID          string
	EventID     string
	SessionName string
	EventType   string
	Payload     []byte
	Attempts    int
	Error       string
	ReceivedAt  time.Time
	NextRetryAt time.Time
	ProcessedAt *time.Time
}
