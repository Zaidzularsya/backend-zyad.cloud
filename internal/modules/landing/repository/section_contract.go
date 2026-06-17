package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type CreateSectionParams struct {
	LandingPageID string
	Key           string
	Type          domain.SectionType
	Name          string
	SortOrder     int
	IsEnabled     bool
	Content       map[string]any
	Style         map[string]any
	CreatedBy     string
}

type UpdateSectionParams struct {
	Name      *string
	IsEnabled *bool
	Content   map[string]any
	Style     map[string]any
	UpdatedBy string
}

type SectionReorderParam struct {
	ID        string
	SortOrder int
}

type SectionRepository interface {
	Create(context.Context, coretenant.Scope, CreateSectionParams) (domain.LandingSection, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.LandingSection, error)
	ListByPage(context.Context, coretenant.Scope, string) ([]domain.LandingSection, error)
	Update(context.Context, coretenant.Scope, string, UpdateSectionParams) (domain.LandingSection, error)
	Reorder(context.Context, coretenant.Scope, string, []SectionReorderParam) error
	Delete(context.Context, coretenant.Scope, string) error
}
