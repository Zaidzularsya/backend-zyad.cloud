package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type FormService interface {
	CreateForm(ctx context.Context, scope coretenant.Scope, params repository.CreateFormParams) (domain.LandingForm, error)
	GetForm(ctx context.Context, scope coretenant.Scope, formID string) (domain.LandingForm, error)
	ListFormsByPage(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingForm, error)
	UpdateForm(ctx context.Context, scope coretenant.Scope, formID string, params repository.UpdateFormParams) (domain.LandingForm, error)
	DeleteForm(ctx context.Context, scope coretenant.Scope, formID string) error

	CreateField(ctx context.Context, scope coretenant.Scope, params repository.CreateFormFieldParams) (domain.LandingFormField, error)
	UpdateField(ctx context.Context, scope coretenant.Scope, fieldID string, params repository.UpdateFormFieldParams) (domain.LandingFormField, error)
	ReorderFields(ctx context.Context, scope coretenant.Scope, formID string, params []repository.FormFieldReorderParam) error
	DeleteField(ctx context.Context, scope coretenant.Scope, fieldID string) error
	ReplaceFields(ctx context.Context, scope coretenant.Scope, formID string, params []repository.CreateFormFieldParams) ([]domain.LandingFormField, error)
}
