package dispatcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/platform/mail"
)

type EmailDispatcher struct {
	mailer mail.Mailer
	now    func() time.Time
}

func NewEmailDispatcher(mailer mail.Mailer) *EmailDispatcher {
	return &EmailDispatcher{
		mailer: mailer,
		now:    time.Now,
	}
}

func (d *EmailDispatcher) Channel() domain.Channel {
	return domain.ChannelEmail
}

func (d *EmailDispatcher) Send(ctx context.Context, message Message) (ProviderResult, error) {
	if strings.TrimSpace(message.Destination) == "" {
		return ProviderResult{}, fmt.Errorf("%w: email destination is required", ErrInvalidMessage)
	}
	if strings.TrimSpace(message.Subject) == "" {
		return ProviderResult{}, fmt.Errorf("%w: email subject is required", ErrInvalidMessage)
	}
	if d.mailer == nil {
		return ProviderResult{}, fmt.Errorf("%w: mailer is required", ErrProviderFailed)
	}

	result, err := d.mailer.Send(ctx, mail.Message{
		To:       message.Destination,
		Subject:  message.Subject,
		Body:     message.Body,
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
