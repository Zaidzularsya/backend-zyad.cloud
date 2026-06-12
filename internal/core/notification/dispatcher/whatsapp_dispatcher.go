package dispatcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/platform/whatsapp"
)

type WhatsAppDispatcher struct {
	client whatsapp.Client
	now    func() time.Time
}

func NewWhatsAppDispatcher(client whatsapp.Client) *WhatsAppDispatcher {
	return &WhatsAppDispatcher{
		client: client,
		now:    time.Now,
	}
}

func (d *WhatsAppDispatcher) Channel() domain.Channel {
	return domain.ChannelWhatsApp
}

func (d *WhatsAppDispatcher) Send(ctx context.Context, message Message) (ProviderResult, error) {
	if strings.TrimSpace(message.Destination) == "" {
		return ProviderResult{}, fmt.Errorf("%w: whatsapp destination is required", ErrInvalidMessage)
	}
	if d.client == nil {
		return ProviderResult{}, fmt.Errorf("%w: whatsapp client is required", ErrProviderFailed)
	}

	result, err := d.client.SendText(ctx, whatsapp.Message{
		To:       message.Destination,
		Text:     message.Body,
		Metadata: message.Metadata,
	})
	if err != nil {
		return ProviderResult{}, fmt.Errorf("%w: %v", ErrProviderFailed, err)
	}

	sentAt := result.SentAt
	if sentAt.IsZero() {
		sentAt = d.now().UTC()
	}

	return ProviderResult{
		Provider:          result.Provider,
		ProviderMessageID: result.MessageID,
		RawResponse:       result.Raw,
		SentAt:            sentAt,
	}, nil
}
