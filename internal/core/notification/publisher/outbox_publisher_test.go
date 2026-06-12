package publisher

import (
	"context"
	"testing"

	"zyad.cloud/internal/core/notification/domain"
)

func TestOutboxPublisherPublish(t *testing.T) {
	store := &fakeOutboxStore{}
	publisher := NewOutboxPublisher(store, 5)

	event, err := publisher.Publish(context.Background(), Event{
		ID:     "00000000-0000-0000-0000-000000000601",
		Type:   "auth.password_reset_requested",
		UserID: "00000000-0000-0000-0000-000000000101",
		Recipient: domain.NotificationRecipient{
			Type:  "user",
			Email: "admin@example.test",
		},
		Payload: map[string]any{"app_name": "Zyad Cloud"},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if event.MaxAttempts != 5 {
		t.Fatalf("MaxAttempts = %d, want 5", event.MaxAttempts)
	}
	if store.event.EventType != "auth.password_reset_requested" {
		t.Fatalf("EventType = %q", store.event.EventType)
	}
	if store.event.Recipient.Email != "admin@example.test" {
		t.Fatalf("Recipient = %#v", store.event.Recipient)
	}
}

func TestOutboxPublisherRejectsMissingType(t *testing.T) {
	publisher := NewOutboxPublisher(&fakeOutboxStore{}, 3)

	_, err := publisher.Publish(context.Background(), Event{})
	if err == nil {
		t.Fatal("Publish() error = nil, want error")
	}
}

func TestOutboxPublisherRequiresStore(t *testing.T) {
	publisher := NewOutboxPublisher(nil, 3)

	_, err := publisher.Publish(context.Background(), Event{Type: "auth.password_reset_requested"})
	if err == nil {
		t.Fatal("Publish() error = nil, want error")
	}
}

type fakeOutboxStore struct {
	event domain.OutboxEvent
}

func (s *fakeOutboxStore) Enqueue(_ context.Context, event *domain.OutboxEvent) error {
	s.event = *event
	return nil
}
