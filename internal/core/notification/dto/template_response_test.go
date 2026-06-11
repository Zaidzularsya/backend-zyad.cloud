package dto

import (
	"testing"
	"time"

	"zyad.cloud/internal/core/notification/domain"
)

func TestNewTemplateResponse(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	template := domain.NotificationTemplate{
		ID:              "template-1",
		Code:            "auth.forgot_password",
		Name:            "Forgot Password",
		Channel:         domain.ChannelEmail,
		Locale:          "id-ID",
		SubjectTemplate: "Reset password",
		BodyTemplate:    "Hello {{name}}",
		AvailableVariables: []domain.NotificationVariable{
			{Key: "name", Description: "Recipient name", Required: true},
		},
		SamplePayload: map[string]any{"name": "Admin"},
		Status:        domain.TemplateStatusActive,
		IsActive:      true,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	response := NewTemplateResponse(template)

	if response.ID != template.ID {
		t.Fatalf("ID = %q, want %q", response.ID, template.ID)
	}
	if response.Channel != string(domain.ChannelEmail) {
		t.Fatalf("Channel = %q, want %q", response.Channel, domain.ChannelEmail)
	}
	if len(response.AvailableVariables) != 1 {
		t.Fatalf("AvailableVariables length = %d, want 1", len(response.AvailableVariables))
	}
	if response.AvailableVariables[0].Key != "name" {
		t.Fatalf("AvailableVariables[0].Key = %q, want name", response.AvailableVariables[0].Key)
	}
}
