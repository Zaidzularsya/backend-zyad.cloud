package dispatcher

import (
	"context"
	"time"

	"zyad.cloud/internal/core/notification/domain"
)

type NoopDispatcher struct {
	channel domain.Channel
	now     func() time.Time
}

func NewNoopDispatcher(channel domain.Channel) *NoopDispatcher {
	if channel == "" {
		channel = domain.ChannelInApp
	}
	return &NoopDispatcher{
		channel: channel,
		now:     time.Now,
	}
}

func (d *NoopDispatcher) Channel() domain.Channel {
	return d.channel
}

func (d *NoopDispatcher) Send(_ context.Context, message Message) (ProviderResult, error) {
	sentAt := d.now().UTC()
	return ProviderResult{
		Provider:          "noop",
		ProviderMessageID: "noop",
		RawResponse: map[string]any{
			"noop":        true,
			"channel":     string(d.channel),
			"destination": message.Destination,
			"subject":     message.Subject,
		},
		SentAt: sentAt,
	}, nil
}
