package dto

import (
	"time"

	"zyad.cloud/internal/core/notification/domain"
)

type TemplateResponse struct {
	ID                 string         `json:"id"`
	Code               string         `json:"code"`
	Name               string         `json:"name"`
	Description        string         `json:"description,omitempty"`
	Channel            string         `json:"channel"`
	Locale             string         `json:"locale"`
	SubjectTemplate    string         `json:"subject_template,omitempty"`
	BodyTemplate       string         `json:"body_template"`
	AvailableVariables []VariableItem `json:"available_variables"`
	SamplePayload      map[string]any `json:"sample_payload"`
	Status             string         `json:"status"`
	IsSystem           bool           `json:"is_system"`
	IsActive           bool           `json:"is_active"`
	Version            int            `json:"version"`
	CreatedBy          string         `json:"created_by,omitempty"`
	UpdatedBy          string         `json:"updated_by,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          *time.Time     `json:"deleted_at,omitempty"`
}

type PreviewTemplateResponse struct {
	Subject string         `json:"subject,omitempty"`
	Body    string         `json:"body"`
	Payload map[string]any `json:"payload,omitempty"`
}

func NewTemplateResponse(template domain.NotificationTemplate) TemplateResponse {
	return TemplateResponse{
		ID:                 template.ID,
		Code:               template.Code,
		Name:               template.Name,
		Description:        template.Description,
		Channel:            string(template.Channel),
		Locale:             template.Locale,
		SubjectTemplate:    template.SubjectTemplate,
		BodyTemplate:       template.BodyTemplate,
		AvailableVariables: NewVariableItems(template.AvailableVariables),
		SamplePayload:      template.SamplePayload,
		Status:             string(template.Status),
		IsSystem:           template.IsSystem,
		IsActive:           template.IsActive,
		Version:            template.Version,
		CreatedBy:          template.CreatedBy,
		UpdatedBy:          template.UpdatedBy,
		CreatedAt:          template.CreatedAt,
		UpdatedAt:          template.UpdatedAt,
		DeletedAt:          template.DeletedAt,
	}
}

func NewTemplateResponses(templates []domain.NotificationTemplate) []TemplateResponse {
	responses := make([]TemplateResponse, 0, len(templates))
	for _, template := range templates {
		responses = append(responses, NewTemplateResponse(template))
	}
	return responses
}
