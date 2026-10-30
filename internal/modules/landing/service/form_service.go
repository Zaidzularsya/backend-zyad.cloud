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
	number, err := NormalizeWhatsAppNumber(params.WhatsAppNumber)
	if err != nil {
		return domain.LandingForm{}, err
	}
	params.WhatsAppNumber = number

	// Klien lama hanya mengirim redirect_url: perlakukan sebagai aksi redirect (sama dengan backfill migration).
	if params.AfterSubmit == "" {
		params.AfterSubmit = domain.AfterSubmitMessage
		if params.RedirectURL != "" {
			params.AfterSubmit = domain.AfterSubmitRedirect
		}
	}
	// Field inti belum dicek di sini: form baru dibuat kosong, lalu diisi lewat ReplaceFields.
	if err := checkAfterSubmit(params.AfterSubmit, params.RedirectURL, params.WhatsAppNumber); err != nil {
		return domain.LandingForm{}, err
	}
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
	if params.WhatsAppNumber != nil {
		number, err := NormalizeWhatsAppNumber(*params.WhatsAppNumber)
		if err != nil {
			return domain.LandingForm{}, err
		}
		params.WhatsAppNumber = &number
	}

	touchesAfterSubmit := params.AfterSubmit != nil || params.WhatsAppNumber != nil || params.RedirectURL != nil
	enablesCRM := params.CreateCRMLead != nil && *params.CreateCRMLead
	if !touchesAfterSubmit && !enablesCRM {
		return s.formRepo.Update(ctx, scope, formID, params)
	}

	current, err := s.formRepo.FindByID(ctx, scope, formID)
	if err != nil {
		return domain.LandingForm{}, err
	}

	if touchesAfterSubmit {
		action, redirect, number := current.AfterSubmit, current.RedirectURL, current.WhatsAppNumber
		if params.RedirectURL != nil {
			redirect = *params.RedirectURL
		}
		if params.WhatsAppNumber != nil {
			number = *params.WhatsAppNumber
		}
		switch {
		case params.AfterSubmit != nil:
			action = *params.AfterSubmit
		case params.RedirectURL != nil && redirect != "" && action == domain.AfterSubmitMessage:
			// Klien lama hanya mengirim redirect_url.
			action = domain.AfterSubmitRedirect
			params.AfterSubmit = &action
		case params.RedirectURL != nil && redirect == "" && action == domain.AfterSubmitRedirect:
			action = domain.AfterSubmitMessage
			params.AfterSubmit = &action
		}
		if err := checkAfterSubmit(action, redirect, number); err != nil {
			return domain.LandingForm{}, err
		}
	}

	if enablesCRM {
		fields, err := s.formRepo.ListFieldsByForm(ctx, scope, formID)
		if err != nil {
			return domain.LandingForm{}, err
		}
		if err := checkCoreFields(true, shapesFromFields(fields)); err != nil {
			return domain.LandingForm{}, err
		}
	}

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
	// 1. Validasi seluruh daftar SEBELUM menghapus apa pun, supaya penolakan tidak meninggalkan form kosong.
	for _, param := range params {
		if !param.Type.IsValid() {
			return nil, ErrInvalidFieldType
		}
	}
	params, err := normalizeFieldParams(params)
	if err != nil {
		return nil, err
	}
	form, err := s.formRepo.FindByID(ctx, scope, formID)
	if err != nil {
		return nil, err
	}
	if err := checkCoreFields(form.CreateCRMLead, shapesFromParams(params)); err != nil {
		return nil, err
	}

	// 2. Fetch existing fields
	existingFields, err := s.formRepo.ListFieldsByForm(ctx, scope, formID)
	if err != nil {
		return nil, err
	}

	// 3. Delete existing fields
	for _, field := range existingFields {
		err := s.formRepo.DeleteField(ctx, scope, field.ID)
		if err != nil {
			return nil, err
		}
	}

	// 4. Create new fields
	var createdFields []domain.LandingFormField
	for _, param := range params {
		param.FormID = formID
		field, err := s.formRepo.CreateField(ctx, scope, param)
		if err != nil {
			return nil, err
		}
		createdFields = append(createdFields, field)
	}

	return createdFields, nil
}
