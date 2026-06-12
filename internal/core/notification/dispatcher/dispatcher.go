package dispatcher

import (
	"context"
	"time"

	"zyad.cloud/internal/core/notification/domain"
)

type Dispatcher interface {
	Channel() domain.Channel
	Send(ctx context.Context, message Message) (ProviderResult, error)
}

type Message struct {
	Channel     domain.Channel
	Destination string
	Subject     string
	Body        string
	Recipient   Recipient
	Metadata    map[string]any
}

type Recipient struct {
	Type   string
	UserID string
	Name   string
	Email  string
	Phone  string
}

type ProviderResult struct {
	Provider          string
	ProviderMessageID string
	RawResponse       map[string]any
	SentAt            time.Time
}
