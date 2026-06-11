package dto

import (
	"time"

	"zyad.cloud/internal/core/notification/domain"
)

type PreferenceResponse struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	OrganizationID string    `json:"organization_id,omitempty"`
	EventType      string    `json:"event_type"`
	Channel        string    `json:"channel"`
	IsEnabled      bool      `json:"is_enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func NewPreferenceResponse(preference domain.NotificationPreference) PreferenceResponse {
	return PreferenceResponse{
		ID:             preference.ID,
		UserID:         preference.UserID,
		OrganizationID: preference.OrganizationID,
		EventType:      preference.EventType,
		Channel:        string(preference.Channel),
		IsEnabled:      preference.IsEnabled,
		CreatedAt:      preference.CreatedAt,
		UpdatedAt:      preference.UpdatedAt,
	}
}

func NewPreferenceResponses(preferences []domain.NotificationPreference) []PreferenceResponse {
	responses := make([]PreferenceResponse, 0, len(preferences))
	for _, preference := range preferences {
		responses = append(responses, NewPreferenceResponse(preference))
	}
	return responses
}
