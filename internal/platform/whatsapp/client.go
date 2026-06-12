package whatsapp

import (
	"context"
	"time"
)

type Message struct {
	To       string
	Text     string
	Metadata map[string]any
}

type Result struct {
	Provider  string
	MessageID string
	Raw       map[string]any
	SentAt    time.Time
}

type Client interface {
	SendText(ctx context.Context, message Message) (Result, error)
}

type NoopClient struct {
	Provider string
}

func NewNoopClient() NoopClient {
	return NoopClient{Provider: "noop"}
}

func (c NoopClient) SendText(_ context.Context, message Message) (Result, error) {
	provider := c.Provider
	if provider == "" {
		provider = "noop"
	}

	return Result{
		Provider:  provider,
		MessageID: "noop",
		Raw: map[string]any{
			"to":   message.To,
			"noop": true,
		},
		SentAt: time.Now().UTC(),
	}, nil
}
