package service

import (
	"context"
	"errors"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

var (
	ErrInvalidFieldType = errors.New("invalid form field type")
	ErrFormNotFound     = errors.New("form not found")
)

type formService struct {
	formRepo repository.FormRepository
}

func NewFormService(formRepo repository.FormRepository) FormService {
	return &formService{
		formRepo: formRepo,
	}
}

func (s *formService) CreateForm(ctx context.Context, scope coretenant.Scope, params repository.CreateFormParams) (domain.LandingForm, error) {
	return s.formRepo.Create(ctx, scope, params)
}

func (s *formService) GetForm(ctx context.Context, scope coretenant.Scope, formID string) (domain.LandingForm, error) {
	form, err := s.formRepo.FindByID(ctx, scope, formID)
	if err != nil {
		return domain.LandingForm{}, err
	}
	
	// Load fields
	fields, err := s.formRepo.ListFieldsByForm(ctx, scope, formID)
	if err == nil {
		form.Fields = fields
	}

	return form, nil
}

func (s *formService) ListFormsByPage(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingForm, error) {
	return s.formRepo.ListByPage(ctx, scope, pageID)
}

func (s *formService) UpdateForm(ctx context.Context, scope coretenant.Scope, formID string, params repository.UpdateFormParams) (domain.LandingForm, error) {
	return s.formRepo.Update(ctx, scope, formID, params)
}

func (s *formService) DeleteForm(ctx context.Context, scope coretenant.Scope, formID string) error {
	return s.formRepo.Delete(ctx, scope, formID)
}

func (s *formService) CreateField(ctx context.Context, scope coretenant.Scope, params repository.CreateFormFieldParams) (domain.LandingFormField, error) {
	if !params.Type.IsValid() {
		return domain.LandingFormField{}, ErrInvalidFieldType
	}

	return s.formRepo.CreateField(ctx, scope, params)
}

func (s *formService) UpdateField(ctx context.Context, scope coretenant.Scope, fieldID string, params repository.UpdateFormFieldParams) (domain.LandingFormField, error) {
	return s.formRepo.UpdateField(ctx, scope, fieldID, params)
}

func (s *formService) ReorderFields(ctx context.Context, scope coretenant.Scope, formID string, params []repository.FormFieldReorderParam) error {
	return s.formRepo.ReorderFields(ctx, scope, formID, params)
}

func (s *formService) DeleteField(ctx context.Context, scope coretenant.Scope, fieldID string) error {
	return s.formRepo.DeleteField(ctx, scope, fieldID)
}
