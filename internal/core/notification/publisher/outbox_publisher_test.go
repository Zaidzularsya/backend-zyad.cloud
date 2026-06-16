package publisher

import (
	"context"
	"testing"

	coreevent "zyad.cloud/internal/core/event"
	"zyad.cloud/internal/core/notification/domain"
	coretenant "zyad.cloud/internal/core/tenant"
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

func TestOutboxPublisherPublishTenantValidatesEnvelope(t *testing.T) {
	store := &fakeOutboxStore{}
	publisher := NewOutboxPublisher(store, 3)
	tenantEvent, err := coreevent.New(publisherTestScope(t), coreevent.Input{
		ID:           "00000000-0000-0000-0000-000000000701",
		Type:         "organization.created",
		ActorUserID:  "00000000-0000-0000-0000-000000000101",
		ResourceType: "organization",
		ResourceID:   "00000000-0000-0000-0000-000000000201",
		Payload:      map[string]any{"name": "Acme"},
	})
	if err != nil {
		t.Fatalf("event.New() error = %v", err)
	}

	_, err = publisher.PublishTenant(
		context.Background(),
		tenantEvent,
		domain.NotificationRecipient{Type: "user", Email: "admin@example.test"},
		"id-ID",
		0,
	)
	if err != nil {
		t.Fatalf("PublishTenant() error = %v", err)
	}
	if store.event.OrganizationID != "00000000-0000-0000-0000-000000000201" ||
		store.event.UserID != "00000000-0000-0000-0000-000000000101" ||
		store.event.Payload["organization_id"] != "00000000-0000-0000-0000-000000000201" {
		t.Fatalf("outbox event = %#v", store.event)
	}
}

type fakeOutboxStore struct {
	event domain.OutboxEvent
}

func (s *fakeOutboxStore) Enqueue(_ context.Context, event *domain.OutboxEvent) error {
	s.event = *event
	return nil
}

func publisherTestScope(t *testing.T) coretenant.Scope {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     "00000000-0000-0000-0000-000000000201",
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceWorker,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	scope, err := coretenant.NewScope(tenantContext)
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	return scope
}
