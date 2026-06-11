package dto

import (
	"time"

	"zyad.cloud/internal/core/notification/domain"
)

type NotificationLogResponse struct {
	ID                     string     `json:"id"`
	EventID                string     `json:"event_id,omitempty"`
	EventType              string     `json:"event_type,omitempty"`
	TemplateID             string     `json:"template_id,omitempty"`
	TemplateCode           string     `json:"template_code,omitempty"`
	TemplateVersion        int        `json:"template_version,omitempty"`
	OrganizationID         string     `json:"organization_id,omitempty"`
	Channel                string     `json:"channel"`
	RecipientType          string     `json:"recipient_type"`
	RecipientUserID        string     `json:"recipient_user_id,omitempty"`
	RecipientNameSnapshot  string     `json:"recipient_name_snapshot,omitempty"`
	RecipientEmailSnapshot string     `json:"recipient_email_snapshot,omitempty"`
	RecipientPhoneSnapshot string     `json:"recipient_phone_snapshot,omitempty"`
	Destination            string     `json:"destination"`
	Subject                string     `json:"subject,omitempty"`
	Body                   string     `json:"body"`
	Status                 string     `json:"status"`
	Provider               string     `json:"provider,omitempty"`
	ProviderMessageID      string     `json:"provider_message_id,omitempty"`
	Attempts               int        `json:"attempts"`
	MaxAttempts            int        `json:"max_attempts"`
	NextRetryAt            *time.Time `json:"next_retry_at,omitempty"`
	ErrorMessage           string     `json:"error_message,omitempty"`
	SentAt                 *time.Time `json:"sent_at,omitempty"`
	FailedAt               *time.Time `json:"failed_at,omitempty"`
	CancelledAt            *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

func NewNotificationLogResponse(log domain.NotificationLog) NotificationLogResponse {
	return NotificationLogResponse{
		ID:                     log.ID,
		EventID:                log.EventID,
		EventType:              log.EventType,
		TemplateID:             log.TemplateID,
		TemplateCode:           log.TemplateCode,
		TemplateVersion:        log.TemplateVersion,
		OrganizationID:         log.OrganizationID,
		Channel:                string(log.Channel),
		RecipientType:          log.RecipientType,
		RecipientUserID:        log.RecipientUserID,
		RecipientNameSnapshot:  log.RecipientNameSnapshot,
		RecipientEmailSnapshot: log.RecipientEmailSnapshot,
		RecipientPhoneSnapshot: log.RecipientPhoneSnapshot,
		Destination:            log.Destination,
		Subject:                log.Subject,
		Body:                   log.Body,
		Status:                 string(log.Status),
		Provider:               log.Provider,
		ProviderMessageID:      log.ProviderMessageID,
		Attempts:               log.Attempts,
		MaxAttempts:            log.MaxAttempts,
		NextRetryAt:            log.NextRetryAt,
		ErrorMessage:           log.ErrorMessage,
		SentAt:                 log.SentAt,
		FailedAt:               log.FailedAt,
		CancelledAt:            log.CancelledAt,
		CreatedAt:              log.CreatedAt,
		UpdatedAt:              log.UpdatedAt,
	}
}

func NewNotificationLogResponses(logs []domain.NotificationLog) []NotificationLogResponse {
	responses := make([]NotificationLogResponse, 0, len(logs))
	for _, log := range logs {
		responses = append(responses, NewNotificationLogResponse(log))
	}
	return responses
}
