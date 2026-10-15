package service

import (
	"context"
	"testing"
	"time"

	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
)

type fakeOwners []repository.Owner

func (o fakeOwners) ListOrganizationOwners(context.Context, string) ([]repository.Owner, error) {
	return o, nil
}

type fakePublisher struct {
	events []notificationpublisher.Event
}

func (p *fakePublisher) Publish(_ context.Context, event notificationpublisher.Event) (notificationdomain.OutboxEvent, error) {
	p.events = append(p.events, event)
	return notificationdomain.OutboxEvent{}, nil
}

func TestOwnerDisconnectNotifierPublishesPerOwner(t *testing.T) {
	publisher := &fakePublisher{}
	notifier := NewOwnerDisconnectNotifier(
		fakeOwners{{UserID: "u1", Name: "Ani", Email: "ani@example.test"}, {UserID: "u2", Name: "Budi", Email: "budi@example.test"}},
		publisher,
		DisconnectNotifierConfig{AppName: "Zyad Cloud", FrontendURL: "https://zyad.online/", Locale: "id-ID", MaxAttempts: 3},
	)
	at := time.Date(2026, 9, 24, 3, 0, 0, 0, time.UTC)
	session := domain.Session{
		ID: "s1", OrganizationID: testOrgID, DisplayName: "Sales", Phone: "6281234567890",
		Status: domain.SessionStatusScanQRCode, LastStatusAt: &at,
	}

	if err := notifier.NotifyDisconnected(context.Background(), session, domain.SessionStatusWorking); err != nil {
		t.Fatalf("NotifyDisconnected() error = %v", err)
	}
	if len(publisher.events) != 2 {
		t.Fatalf("events = %d, want 2", len(publisher.events))
	}
	event := publisher.events[0]
	if event.Type != EventSessionDisconnected || event.OrganizationID != testOrgID || event.Recipient.Email != "ani@example.test" {
		t.Fatalf("event = %+v", event)
	}
	want := map[string]any{
		"app_name":        "Zyad Cloud",
		"user_name":       "Ani",
		"session_name":    "Sales +6281234567890",
		"status":          "perangkat logout, perlu pairing ulang",
		"occurred_at":     "2026-09-24 10:00 WIB",
		"connections_url": "https://zyad.online/app/whatsapp",
	}
	for key, value := range want {
		if event.Payload[key] != value {
			t.Errorf("payload[%s] = %v, want %v", key, event.Payload[key], value)
		}
	}
	if publisher.events[1].Payload["user_name"] != "Budi" {
		t.Fatalf("second owner payload = %+v", publisher.events[1].Payload)
	}
}
