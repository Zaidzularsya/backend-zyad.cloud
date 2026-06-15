package consumer

import (
	"context"
	"strings"
	"time"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/service"
	coretenant "zyad.cloud/internal/core/tenant"
)

type OutboxStore interface {
	ClaimPending(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	MarkSucceeded(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, errorMessage string, nextRetryAt *time.Time) error
	MarkDead(ctx context.Context, id string, errorMessage string) error
}

type WorkerTenantResolver interface {
	ResolveWorkerOrganization(
		context.Context,
		string,
		string,
	) (coretenant.Context, error)
}

type OutboxWorker struct {
	store           OutboxStore
	consumer        *NotificationEventConsumer
	tenantResolver  WorkerTenantResolver
	serviceIdentity string
	now             func() time.Time
}

func NewOutboxWorker(store OutboxStore, consumer *NotificationEventConsumer) *OutboxWorker {
	return &OutboxWorker{
		store:    store,
		consumer: consumer,
		now:      time.Now,
	}
}

func (w *OutboxWorker) SetTenantResolver(
	resolver WorkerTenantResolver,
	serviceIdentity string,
) {
	w.tenantResolver = resolver
	w.serviceIdentity = strings.TrimSpace(serviceIdentity)
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
	if strings.TrimSpace(outboxEvent.OrganizationID) != "" {
		if w.tenantResolver == nil {
			return w.failOutboxEvent(
				ctx,
				outboxEvent,
				Result{
					EventID:   outboxEvent.ID,
					EventType: outboxEvent.EventType,
					Error:     "worker tenant resolver is required",
				},
			)
		}
		tenantContext, err := w.tenantResolver.ResolveWorkerOrganization(
			ctx,
			outboxEvent.OrganizationID,
			w.serviceIdentity,
		)
		if err != nil {
			return w.failOutboxEvent(
				ctx,
				outboxEvent,
				Result{
					EventID:   outboxEvent.ID,
					EventType: outboxEvent.EventType,
					Error:     err.Error(),
				},
			)
		}
		ctx = coretenant.WithContext(ctx, tenantContext)
	}

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
	return w.failOutboxEvent(ctx, outboxEvent, result)
}

func (w *OutboxWorker) failOutboxEvent(
	ctx context.Context,
	outboxEvent domain.OutboxEvent,
	result Result,
) (Result, error) {
	if outboxEvent.Attempts+1 >= outboxEvent.MaxAttempts {
		return result, w.store.MarkDead(ctx, outboxEvent.ID, result.Error)
	}

	nextRetryAt := w.now().UTC().Add(time.Minute)
	return result, w.store.MarkFailed(ctx, outboxEvent.ID, result.Error, &nextRetryAt)
}
