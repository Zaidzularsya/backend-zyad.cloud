package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/service"
)

func TestOutboxWorkerMarksKnownEventSucceeded(t *testing.T) {
	store := &fakeOutboxStore{
		events: []domain.OutboxEvent{
			{
				ID:          "00000000-0000-0000-0000-000000000001",
				EventType:   "auth.password_reset_requested",
				Payload:     passwordResetPayload(),
				MaxAttempts: 3,
			},
		},
	}
	worker := NewOutboxWorker(store, NewNotificationEventConsumer(service.NewNotificationRuleService(), &fakeSender{}))

	results, err := worker.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if len(results) != 1 || !results[0].Sent {
		t.Fatalf("results = %#v", results)
	}
	if store.succeededID != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("succeededID = %q", store.succeededID)
	}
}

func TestOutboxWorkerMarksUnknownEventSucceeded(t *testing.T) {
	store := &fakeOutboxStore{
		events: []domain.OutboxEvent{
			{
				ID:          "00000000-0000-0000-0000-000000000002",
				EventType:   "unknown.event",
				MaxAttempts: 3,
			},
		},
	}
	worker := NewOutboxWorker(store, NewNotificationEventConsumer(service.NewNotificationRuleService(), &fakeSender{}))

	results, err := worker.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if len(results) != 1 || !results[0].Ignored {
		t.Fatalf("results = %#v", results)
	}
	if store.succeededID != "00000000-0000-0000-0000-000000000002" {
		t.Fatalf("succeededID = %q", store.succeededID)
	}
}

func TestOutboxWorkerMarksFailedForRetryableError(t *testing.T) {
	store := &fakeOutboxStore{
		events: []domain.OutboxEvent{
			{
				ID:          "00000000-0000-0000-0000-000000000003",
				EventType:   "auth.password_reset_requested",
				Payload:     passwordResetPayload(),
				Attempts:    1,
				MaxAttempts: 3,
			},
		},
	}
	worker := NewOutboxWorker(store, NewNotificationEventConsumer(service.NewNotificationRuleService(), &fakeSender{err: errors.New("send failed")}))
	worker.now = func() time.Time {
		return time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	}

	results, err := worker.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if len(results) != 1 || results[0].Error == "" {
		t.Fatalf("results = %#v", results)
	}
	if store.failedID != "00000000-0000-0000-0000-000000000003" {
		t.Fatalf("failedID = %q", store.failedID)
	}
	if store.nextRetryAt == nil || !store.nextRetryAt.Equal(time.Date(2026, 6, 12, 10, 1, 0, 0, time.UTC)) {
		t.Fatalf("nextRetryAt = %v", store.nextRetryAt)
	}
}

func TestOutboxWorkerMarksDeadAtMaxAttempts(t *testing.T) {
	store := &fakeOutboxStore{
		events: []domain.OutboxEvent{
			{
				ID:          "00000000-0000-0000-0000-000000000004",
				EventType:   "auth.password_reset_requested",
				Payload:     passwordResetPayload(),
				Attempts:    2,
				MaxAttempts: 3,
			},
		},
	}
	worker := NewOutboxWorker(store, NewNotificationEventConsumer(service.NewNotificationRuleService(), &fakeSender{err: errors.New("send failed")}))

	_, err := worker.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if store.deadID != "00000000-0000-0000-0000-000000000004" {
		t.Fatalf("deadID = %q", store.deadID)
	}
}

func passwordResetPayload() map[string]any {
	return map[string]any{
		"app_name":   "Zyad Cloud",
		"user_id":    "00000000-0000-0000-0000-000000000101",
		"user_name":  "Admin",
		"user_email": "admin@example.test",
		"reset_url":  "https://app.example.test/reset",
		"expired_at": "2026-06-12 10:00 WIB",
	}
}

type fakeOutboxStore struct {
	events      []domain.OutboxEvent
	succeededID string
	failedID    string
	deadID      string
	nextRetryAt *time.Time
}

func (s *fakeOutboxStore) ClaimPending(context.Context, int) ([]domain.OutboxEvent, error) {
	return s.events, nil
}

func (s *fakeOutboxStore) MarkSucceeded(_ context.Context, id string) error {
	s.succeededID = id
	return nil
}

func (s *fakeOutboxStore) MarkFailed(_ context.Context, id string, _ string, nextRetryAt *time.Time) error {
	s.failedID = id
	s.nextRetryAt = nextRetryAt
	return nil
}

func (s *fakeOutboxStore) MarkDead(_ context.Context, id string, _ string) error {
	s.deadID = id
	return nil
}
