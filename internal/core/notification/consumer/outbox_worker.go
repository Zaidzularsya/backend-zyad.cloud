package consumer

import (
	"context"
	"time"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/service"
)

type OutboxStore interface {
	ClaimPending(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	MarkSucceeded(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, errorMessage string, nextRetryAt *time.Time) error
	MarkDead(ctx context.Context, id string, errorMessage string) error
}

type OutboxWorker struct {
	store    OutboxStore
	consumer *NotificationEventConsumer
	now      func() time.Time
}

func NewOutboxWorker(store OutboxStore, consumer *NotificationEventConsumer) *OutboxWorker {
	return &OutboxWorker{
		store:    store,
		consumer: consumer,
		now:      time.Now,
	}
}

func (w *OutboxWorker) RunOnce(ctx context.Context, limit int) ([]Result, error) {
	events, err := w.store.ClaimPending(ctx, limit)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(events))
	for _, outboxEvent := range events {
		result, err := w.consumeOutboxEvent(ctx, outboxEvent)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}

	return results, nil
}

func (w *OutboxWorker) consumeOutboxEvent(ctx context.Context, outboxEvent domain.OutboxEvent) (Result, error) {
	event := service.NotificationEvent{
		ID:             outboxEvent.ID,
		Type:           outboxEvent.EventType,
		OrganizationID: outboxEvent.OrganizationID,
		UserID:         outboxEvent.UserID,
		Recipient:      outboxEvent.Recipient,
		Payload:        outboxEvent.Payload,
		Locale:         outboxEvent.Locale,
	}

	result, err := w.consumer.Consume(ctx, event)
	if err != nil {
		return result, err
	}
	if result.Sent || result.Ignored {
		return result, w.store.MarkSucceeded(ctx, outboxEvent.ID)
	}
	if result.Error == "" {
		return result, w.store.MarkSucceeded(ctx, outboxEvent.ID)
	}

	if outboxEvent.Attempts+1 >= outboxEvent.MaxAttempts {
		return result, w.store.MarkDead(ctx, outboxEvent.ID, result.Error)
	}

	nextRetryAt := w.now().UTC().Add(time.Minute)
	return result, w.store.MarkFailed(ctx, outboxEvent.ID, result.Error, &nextRetryAt)
}
