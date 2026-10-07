package service

import (
	"context"
	"errors"
	"sort"

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
	forms, err := s.formRepo.ListByPage(ctx, scope, pageID)
	if err != nil {
		return nil, err
	}

	// Admin editors need every form (active or not) together with its fields.
	// One query per form; forms per page are few, so no batch method is needed.
	for i := range forms {
		fields, err := s.formRepo.ListFieldsByForm(ctx, scope, forms[i].ID)
		if err != nil {
			return nil, err
		}
		if fields == nil {
			fields = []domain.LandingFormField{}
		}
		sort.SliceStable(fields, func(a, b int) bool { return fields[a].SortOrder < fields[b].SortOrder })
		forms[i].Fields = fields
	}

	return forms, nil
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

func (s *formService) ReplaceFields(ctx context.Context, scope coretenant.Scope, formID string, params []repository.CreateFormFieldParams) ([]domain.LandingFormField, error) {
	// 1. Fetch existing fields
	existingFields, err := s.formRepo.ListFieldsByForm(ctx, scope, formID)
	if err != nil {
		return nil, err
	}

	// 2. Delete existing fields
	for _, field := range existingFields {
		err := s.formRepo.DeleteField(ctx, scope, field.ID)
		if err != nil {
			return nil, err
		}
	}

	// 3. Create new fields
	var createdFields []domain.LandingFormField
	for _, param := range params {
		param.FormID = formID
		if !param.Type.IsValid() {
			return nil, ErrInvalidFieldType
		}
		field, err := s.formRepo.CreateField(ctx, scope, param)
		if err != nil {
			return nil, err
		}
		createdFields = append(createdFields, field)
	}

	return createdFields, nil
}
