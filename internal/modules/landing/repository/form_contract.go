package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type CreateFormParams struct {
	LandingPageID  string
	Name           string
	Key            string
	Description    string
	SubmitLabel    string
	SuccessMessage string
	RedirectURL    string
	IsActive       bool
	CreatedBy      string
}

type UpdateFormParams struct {
	Name           *string
	Description    *string
	SubmitLabel    *string
	SuccessMessage *string
	RedirectURL    *string
	IsActive       *bool
	UpdatedBy      string
}

type CreateFormFieldParams struct {
	FormID      string
	Key         string
	Type        domain.FormFieldType
	Label       string
	Placeholder string
	Options     []string
	Validation  map[string]any
	IsRequired  bool
	SortOrder   int
}

type UpdateFormFieldParams struct {
	Label       *string
	Placeholder *string
	Options     []string
	Validation  map[string]any
	IsRequired  *bool
}

type FormFieldReorderParam struct {
	ID        string
	SortOrder int
}

type FormRepository interface {
	// Form Operations
	Create(context.Context, coretenant.Scope, CreateFormParams) (domain.LandingForm, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.LandingForm, error)
	FindByKey(context.Context, coretenant.Scope, string, string) (domain.LandingForm, error)
	ListByPage(context.Context, coretenant.Scope, string) ([]domain.LandingForm, error)
	Update(context.Context, coretenant.Scope, string, UpdateFormParams) (domain.LandingForm, error)
	Delete(context.Context, coretenant.Scope, string) error

	// Form Field Operations
	CreateField(context.Context, coretenant.Scope, CreateFormFieldParams) (domain.LandingFormField, error)
	FindFieldByID(context.Context, coretenant.Scope, string) (domain.LandingFormField, error)
	ListFieldsByForm(context.Context, coretenant.Scope, string) ([]domain.LandingFormField, error)
	UpdateField(context.Context, coretenant.Scope, string, UpdateFormFieldParams) (domain.LandingFormField, error)
	ReorderFields(context.Context, coretenant.Scope, string, []FormFieldReorderParam) error
	DeleteField(context.Context, coretenant.Scope, string) error
}
