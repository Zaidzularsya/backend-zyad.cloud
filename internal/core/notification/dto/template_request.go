package dto

type CreateTemplateRequest struct {
	Code               string         `json:"code" binding:"required,min=3,max=150"`
	Name               string         `json:"name" binding:"required,min=3,max=150"`
	Description        string         `json:"description" binding:"max=1000"`
	Channel            string         `json:"channel" binding:"required,oneof=email whatsapp in_app discord"`
	Locale             string         `json:"locale" binding:"omitempty,max=20"`
	SubjectTemplate    string         `json:"subject_template"`
	BodyTemplate       string         `json:"body_template" binding:"required"`
	AvailableVariables []VariableItem `json:"available_variables"`
	SamplePayload      map[string]any `json:"sample_payload"`
	IsSystem           bool           `json:"is_system"`
	IsActive           bool           `json:"is_active"`
	Version            int            `json:"version" binding:"omitempty,min=1"`
}

type UpdateTemplateRequest struct {
	Name               *string        `json:"name" binding:"omitempty,min=3,max=150"`
	Description        *string        `json:"description" binding:"omitempty,max=1000"`
	Locale             *string        `json:"locale" binding:"omitempty,max=20"`
	SubjectTemplate    *string        `json:"subject_template"`
	BodyTemplate       *string        `json:"body_template" binding:"omitempty"`
	AvailableVariables []VariableItem `json:"available_variables"`
	SamplePayload      map[string]any `json:"sample_payload"`
	IsActive           *bool          `json:"is_active"`
}

type PreviewTemplateRequest struct {
	Payload map[string]any `json:"payload"`
	Locale  string         `json:"locale" binding:"omitempty,max=20"`
}

type SendNotificationRequest struct {
	EventID        string                `json:"event_id"`
	EventType      string                `json:"event_type" binding:"required,max=150"`
	TemplateCode   string                `json:"template_code" binding:"required,max=150"`
	OrganizationID string                `json:"organization_id"`
	Channel        string                `json:"channel" binding:"required,oneof=email whatsapp in_app discord"`
	UserID         string                `json:"user_id"`
	Recipient      NotificationRecipient `json:"recipient" binding:"required"`
	Payload        map[string]any        `json:"payload"`
	Locale         string                `json:"locale" binding:"omitempty,max=20"`
}

type PreferenceRequest struct {
	EventType string `json:"event_type" binding:"required,max=150"`
	Channel   string `json:"channel" binding:"required,oneof=email whatsapp in_app discord"`
	IsEnabled bool   `json:"is_enabled"`
}

type BulkPreferenceRequest struct {
	Preferences []PreferenceRequest `json:"preferences" binding:"required"`
}
