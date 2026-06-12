package consumer

import (
	"context"
	"errors"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/service"
)

type RuleMapper interface {
	BuildNotification(ctx context.Context, event service.NotificationEvent) (domain.Notification, error)
}

type NotificationSender interface {
	SendByTemplate(ctx context.Context, notification domain.Notification) (domain.NotificationLog, error)
}

type Result struct {
	EventID   string
	EventType string
	Ignored   bool
	Sent      bool
	Log       domain.NotificationLog
	Error     string
}

type NotificationEventConsumer struct {
	rules  RuleMapper
	sender NotificationSender
}

func NewNotificationEventConsumer(rules RuleMapper, sender NotificationSender) *NotificationEventConsumer {
	return &NotificationEventConsumer{
		rules:  rules,
		sender: sender,
	}
}

func (c *NotificationEventConsumer) Consume(ctx context.Context, event service.NotificationEvent) (Result, error) {
	result := Result{
		EventID:   event.ID,
		EventType: event.Type,
	}

	if c.rules == nil {
		return result, coreerrors.New("NOTIFICATION_RULE_MAPPER_REQUIRED", "notification rule mapper is required", http.StatusInternalServerError)
	}
	if c.sender == nil {
		return result, coreerrors.New("NOTIFICATION_SENDER_REQUIRED", "notification sender is required", http.StatusInternalServerError)
	}

	notification, err := c.rules.BuildNotification(ctx, event)
	if err != nil {
		if isUnknownRule(err) {
			result.Ignored = true
			return result, nil
		}
		result.Error = err.Error()
		return result, nil
	}

	log, err := c.sender.SendByTemplate(ctx, notification)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Sent = true
	result.Log = log
	return result, nil
}

func isUnknownRule(err error) bool {
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) {
		return false
	}
	return appErr.Code == "NOTIFICATION_RULE_NOT_FOUND"
}
