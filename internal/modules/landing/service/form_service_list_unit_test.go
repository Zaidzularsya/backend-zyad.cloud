package service_test

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

type listFormsRepoStub struct {
	repository.FormRepository
	forms  []domain.LandingForm
	fields map[string][]domain.LandingFormField
}

func (s *listFormsRepoStub) ListByPage(context.Context, coretenant.Scope, string) ([]domain.LandingForm, error) {
	return s.forms, nil
}

func (s *listFormsRepoStub) ListFieldsByForm(_ context.Context, _ coretenant.Scope, formID string) ([]domain.LandingFormField, error) {
	return s.fields[formID], nil
}

func TestListFormsByPageIncludesOrderedFields(t *testing.T) {
	repo := &listFormsRepoStub{
		forms: []domain.LandingForm{
			{ID: "f1", IsActive: true},
			{ID: "f2", IsActive: false},
			{ID: "f3", IsActive: true},
		},
		fields: map[string][]domain.LandingFormField{
			"f1": {{ID: "a2", SortOrder: 2}, {ID: "a1", SortOrder: 1}},
			"f2": {{ID: "b1", SortOrder: 1}},
		},
	}
	svc := service.NewFormService(repo)

	forms, err := svc.ListFormsByPage(context.Background(), coretenant.Scope{}, "p1")
	if err != nil {
		t.Fatalf("ListFormsByPage: %v", err)
	}
	if len(forms) != 3 {
		t.Fatalf("want 3 forms (inactive included), got %d", len(forms))
	}
	if got := forms[0].Fields; len(got) != 2 || got[0].ID != "a1" || got[1].ID != "a2" {
		t.Fatalf("f1 fields not ordered by SortOrder: %+v", got)
	}
	if got := forms[1].Fields; len(got) != 1 || got[0].ID != "b1" {
		t.Fatalf("inactive form f2 fields missing: %+v", got)
	}
	if forms[2].Fields == nil || len(forms[2].Fields) != 0 {
		t.Fatalf("form without fields must have empty non-nil Fields, got %#v", forms[2].Fields)
	}
}
