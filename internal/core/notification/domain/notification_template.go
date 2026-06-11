package domain

import "time"

type TemplateStatus string

const (
	TemplateStatusDraft    TemplateStatus = "draft"
	TemplateStatusActive   TemplateStatus = "active"
	TemplateStatusInactive TemplateStatus = "inactive"
	TemplateStatusArchived TemplateStatus = "archived"
)

type NotificationTemplate struct {
	ID                 string
	Code               string
	Name               string
	Description        string
	Channel            Channel
	Locale             string
	SubjectTemplate    string
	BodyTemplate       string
	AvailableVariables []NotificationVariable
	SamplePayload      map[string]any
	Status             TemplateStatus
	IsSystem           bool
	IsActive           bool
	Version            int
	CreatedBy          string
	UpdatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}

func (s TemplateStatus) IsValid() bool {
	switch s {
	case TemplateStatusDraft,
		TemplateStatusActive,
		TemplateStatusInactive,
		TemplateStatusArchived:
		return true
	default:
		return false
	}
}

func (t NotificationTemplate) IsDeleted() bool {
	return t.DeletedAt != nil
}

func (t NotificationTemplate) CanBeUsed() bool {
	return !t.IsDeleted() && t.IsActive && t.Status == TemplateStatusActive
}
