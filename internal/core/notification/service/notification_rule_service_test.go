package service

import (
	"context"
	"errors"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/notification/domain"
)

func TestNotificationRuleServiceBuildPasswordResetNotification(t *testing.T) {
	service := NewNotificationRuleService()

	notification, err := service.BuildNotification(context.Background(), NotificationEvent{
		ID:     "event-1",
		Type:   "auth.password_reset_requested",
		UserID: "user-1",
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
		t.Fatalf("BuildNotification() error = %v", err)
	}

	if notification.TemplateCode != "auth.password_reset" {
		t.Fatalf("TemplateCode = %q, want auth.password_reset", notification.TemplateCode)
	}
	if notification.Channel != domain.ChannelEmail {
		t.Fatalf("Channel = %q, want %q", notification.Channel, domain.ChannelEmail)
	}
	if notification.Recipient.UserID != "user-1" || notification.Recipient.Email != "admin@example.test" {
		t.Fatalf("Recipient = %#v", notification.Recipient)
	}
	if notification.Locale != "id-ID" {
		t.Fatalf("Locale = %q, want id-ID", notification.Locale)
	}
}

func TestNotificationRuleServiceBuildPasswordChangedNotification(t *testing.T) {
	service := NewNotificationRuleService()

	notification, err := service.BuildNotification(context.Background(), NotificationEvent{
		ID:     "event-2",
		Type:   "auth.password_changed",
		UserID: "user-1",
		Payload: map[string]any{
			"app_name":   "Zyad Cloud",
			"user_id":    "user-1",
			"user_name":  "Admin",
			"user_email": "admin@example.test",
			"changed_at": "2026-06-12T10:00:00Z",
		},
	})
	if err != nil {
		t.Fatalf("BuildNotification() error = %v", err)
	}
	if notification.TemplateCode != "auth.password_changed" {
		t.Fatalf("TemplateCode = %q, want auth.password_changed", notification.TemplateCode)
	}
	if notification.Recipient.Email != "admin@example.test" {
		t.Fatalf("Recipient.Email = %q", notification.Recipient.Email)
	}
}

func TestNotificationRuleServiceBuildInvitationNotification(t *testing.T) {
	service := NewNotificationRuleService()

	notification, err := service.BuildNotification(context.Background(), NotificationEvent{
		ID:             "event-2",
		Type:           "user.invited",
		OrganizationID: "org-1",
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
		t.Fatalf("BuildNotification() error = %v", err)
	}

	if notification.TemplateCode != "user.invitation" {
		t.Fatalf("TemplateCode = %q, want user.invitation", notification.TemplateCode)
	}
	if notification.Channel != domain.ChannelEmail {
		t.Fatalf("Channel = %q, want %q", notification.Channel, domain.ChannelEmail)
	}
	if notification.Recipient.Email != "member@example.test" {
		t.Fatalf("Recipient.Email = %q", notification.Recipient.Email)
	}
	if notification.OrganizationID != "org-1" {
		t.Fatalf("OrganizationID = %q, want org-1", notification.OrganizationID)
	}
}

func TestNotificationRuleServiceUnknownEvent(t *testing.T) {
	service := NewNotificationRuleService()

	_, err := service.BuildNotification(context.Background(), NotificationEvent{
		Type: "unknown.event",
	})
	if err == nil {
		t.Fatal("BuildNotification() error = nil, want error")
	}

	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error type = %T, want AppError", err)
	}
	if appErr.Code != "NOTIFICATION_RULE_NOT_FOUND" {
		t.Fatalf("error code = %q, want NOTIFICATION_RULE_NOT_FOUND", appErr.Code)
	}
}
