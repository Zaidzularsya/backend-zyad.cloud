package publisher

import (
	"context"
	"net/http"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/notification/domain"
)

const defaultMaxAttempts = 3

type OutboxStore interface {
	Enqueue(ctx context.Context, event *domain.OutboxEvent) error
}

type Event struct {
	ID             string
	Type           string
	OrganizationID string
	UserID         string
	Recipient      domain.NotificationRecipient
	Payload        map[string]any
	Locale         string
	MaxAttempts    int
}

type OutboxPublisher struct {
	store              OutboxStore
	defaultMaxAttempts int
}

func NewOutboxPublisher(store OutboxStore, defaultAttempts int) *OutboxPublisher {
	if defaultAttempts <= 0 {
		defaultAttempts = defaultMaxAttempts
	}
	return &OutboxPublisher{
		store:              store,
		defaultMaxAttempts: defaultAttempts,
	}
}

func (p *OutboxPublisher) Publish(ctx context.Context, event Event) (domain.OutboxEvent, error) {
	if p.store == nil {
		return domain.OutboxEvent{}, coreerrors.New("NOTIFICATION_OUTBOX_STORE_REQUIRED", "notification outbox store is required", http.StatusInternalServerError)
	}
	if strings.TrimSpace(event.Type) == "" {
		return domain.OutboxEvent{}, coreerrors.New("NOTIFICATION_EVENT_TYPE_REQUIRED", "notification event type is required", http.StatusBadRequest)
	}

	maxAttempts := event.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = p.defaultMaxAttempts
	}

	outboxEvent := domain.OutboxEvent{
		ID:             event.ID,
		EventType:      event.Type,
		OrganizationID: event.OrganizationID,
		UserID:         event.UserID,
		Recipient:      event.Recipient,
		Payload:        event.Payload,
		Locale:         event.Locale,
		MaxAttempts:    maxAttempts,
	}
	if err := p.store.Enqueue(ctx, &outboxEvent); err != nil {
		return domain.OutboxEvent{}, err
	}

	return outboxEvent, nil
}
