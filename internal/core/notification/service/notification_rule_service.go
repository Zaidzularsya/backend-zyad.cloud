package service

import (
	"context"
	"net/http"
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/notification/domain"
)

const defaultRuleLocale = "id-ID"

type NotificationEvent struct {
	ID             string
	Type           string
	OrganizationID string
	UserID         string
	Recipient      domain.NotificationRecipient
	Payload        map[string]any
	Locale         string
}

type NotificationRule struct {
	EventType    string
	TemplateCode string
	Channel      domain.Channel
	Locale       string
}

type NotificationRuleService struct {
	rules map[string]NotificationRule
}

func NewNotificationRuleService() *NotificationRuleService {
	return &NotificationRuleService{rules: defaultNotificationRules()}
}

func (s *NotificationRuleService) GetRule(_ context.Context, eventType string) (NotificationRule, error) {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return NotificationRule{}, coreerrors.New("NOTIFICATION_EVENT_TYPE_REQUIRED", "notification event type is required", http.StatusBadRequest)
	}

	rule, ok := s.rules[eventType]
	if !ok {
		return NotificationRule{}, coreerrors.New("NOTIFICATION_RULE_NOT_FOUND", "notification rule not found", http.StatusNotFound)
	}
	return rule, nil
}

func (s *NotificationRuleService) BuildNotification(ctx context.Context, event NotificationEvent) (domain.Notification, error) {
	rule, err := s.GetRule(ctx, event.Type)
	if err != nil {
		return domain.Notification{}, err
	}

	locale := strings.TrimSpace(event.Locale)
	if locale == "" {
		locale = rule.Locale
	}
	if locale == "" {
		locale = defaultRuleLocale
	}

	recipient := event.Recipient
	if strings.TrimSpace(recipient.UserID) == "" {
		recipient.UserID = firstNonEmpty(event.UserID, payloadString(event.Payload, "user_id"), payloadString(event.Payload, "invitee_user_id"))
	}
	if strings.TrimSpace(recipient.Name) == "" {
		recipient.Name = firstNonEmpty(payloadString(event.Payload, "user_name"), payloadString(event.Payload, "invitee_name"))
	}
	if strings.TrimSpace(recipient.Email) == "" {
		recipient.Email = firstNonEmpty(payloadString(event.Payload, "email"), payloadString(event.Payload, "user_email"), payloadString(event.Payload, "invitee_email"))
	}
	if strings.TrimSpace(recipient.Phone) == "" {
		recipient.Phone = firstNonEmpty(payloadString(event.Payload, "phone"), payloadString(event.Payload, "user_phone"))
	}
	if strings.TrimSpace(recipient.Type) == "" {
		recipient.Type = "user"
	}

	return domain.Notification{
		EventID:        event.ID,
		EventType:      event.Type,
		TemplateCode:   rule.TemplateCode,
		OrganizationID: event.OrganizationID,
		Channel:        rule.Channel,
		UserID:         firstNonEmpty(event.UserID, recipient.UserID),
		Recipient:      recipient,
		Payload:        event.Payload,
		Locale:         locale,
	}, nil
}

func defaultNotificationRules() map[string]NotificationRule {
	return map[string]NotificationRule{
		"auth.password_reset_requested": {
			EventType:    "auth.password_reset_requested",
			TemplateCode: "auth.password_reset",
			Channel:      domain.ChannelEmail,
			Locale:       defaultRuleLocale,
		},
		"auth.password_changed": {
			EventType:    "auth.password_changed",
			TemplateCode: "auth.password_changed",
			Channel:      domain.ChannelEmail,
			Locale:       defaultRuleLocale,
		},
		"user.invited": {
			EventType:    "user.invited",
			TemplateCode: "user.invitation",
			Channel:      domain.ChannelEmail,
			Locale:       defaultRuleLocale,
		},
		"user.invitation_created": {
			EventType:    "user.invitation_created",
			TemplateCode: "user.invitation",
			Channel:      domain.ChannelEmail,
			Locale:       defaultRuleLocale,
		},
	}
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok {
		return ""
	}
	str, _ := value.(string)
	return strings.TrimSpace(str)
}
