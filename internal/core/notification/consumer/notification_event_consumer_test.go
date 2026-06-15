package consumer

import (
	"context"
	"errors"
	"testing"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/service"
	coretenant "zyad.cloud/internal/core/tenant"
)

func TestNotificationEventConsumerConsumesKnownEvent(t *testing.T) {
	sender := &fakeSender{}
	consumer := NewNotificationEventConsumer(service.NewNotificationRuleService(), sender)

	result, err := consumer.Consume(context.Background(), service.NotificationEvent{
		ID:   "event-1",
		Type: "auth.password_reset_requested",
		Payload: map[string]any{
			"app_name":   "Zyad Cloud",
			"user_id":    "user-1",
			"user_name":  "Admin",
			"user_email": "admin@example.test",
			"reset_url":  "https://app.example.test/reset",
			"expired_at": "2026-06-12 10:00 WIB",
		},
	})
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}

	if !result.Sent {
		t.Fatalf("result.Sent = false, result = %#v", result)
	}
	if result.Ignored {
		t.Fatal("result.Ignored = true, want false")
	}
	if sender.calls != 1 {
		t.Fatalf("sender.calls = %d, want 1", sender.calls)
	}
	if sender.notifications[0].TemplateCode != "auth.password_reset" {
		t.Fatalf("TemplateCode = %q", sender.notifications[0].TemplateCode)
	}
}

func TestNotificationEventConsumerIgnoresUnknownEvent(t *testing.T) {
	sender := &fakeSender{}
	consumer := NewNotificationEventConsumer(service.NewNotificationRuleService(), sender)

	result, err := consumer.Consume(context.Background(), service.NotificationEvent{
		ID:   "event-2",
		Type: "unknown.event",
	})
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}

	if !result.Ignored {
		t.Fatalf("result.Ignored = false, result = %#v", result)
	}
	if result.Sent {
		t.Fatal("result.Sent = true, want false")
	}
	if sender.calls != 0 {
		t.Fatalf("sender.calls = %d, want 0", sender.calls)
	}
}

func TestNotificationEventConsumerCapturesSenderError(t *testing.T) {
	consumer := NewNotificationEventConsumer(service.NewNotificationRuleService(), &fakeSender{
		err: errors.New("provider unavailable"),
	})

	result, err := consumer.Consume(context.Background(), service.NotificationEvent{
		ID:   "event-3",
		Type: "user.invited",
		Payload: map[string]any{
			"app_name":          "Zyad Cloud",
			"inviter_name":      "Owner",
			"invitee_email":     "member@example.test",
			"organization_name": "Acme Network",
			"invitation_url":    "https://app.example.test/invitations/accept",
			"expired_at":        "2026-06-19 10:00 WIB",
		},
	})
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}

	if result.Error == "" {
		t.Fatalf("result.Error is empty, result = %#v", result)
	}
	if result.Sent {
		t.Fatal("result.Sent = true, want false")
	}
}

type fakeSender struct {
	err                  error
	calls                int
	notifications        []domain.Notification
	tenantOrganizationID string
}

func (s *fakeSender) SendByTemplate(ctx context.Context, notification domain.Notification) (domain.NotificationLog, error) {
	s.calls++
	s.notifications = append(s.notifications, notification)
	if tenantContext, ok := coretenant.FromContext(ctx); ok {
		s.tenantOrganizationID = tenantContext.OrganizationID()
	}
	if s.err != nil {
		return domain.NotificationLog{}, s.err
	}
	return domain.NotificationLog{
		ID:           "log-1",
		EventID:      notification.EventID,
		EventType:    notification.EventType,
		TemplateCode: notification.TemplateCode,
		Channel:      notification.Channel,
		Status:       domain.LogStatusSent,
	}, nil
}
