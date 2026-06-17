//go:build integration

package service_test

import (
	"context"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestFormServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	pageRepo := repository.NewPageRepository(db)
	formRepo := repository.NewFormRepository(db)
	formService := service.NewFormService(formRepo)

	// Create Page
	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Form Page",
		Title:      "Form Title",
		Slug:       strings.ReplaceAll(testutil.UniqueCode("form-page-"), ".", "-"),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
	})
	if err != nil {
		t.Fatalf("Create page: %v", err)
	}

	// 1. Create Form
	form, err := formService.CreateForm(ctx, tenants.A.Scope, repository.CreateFormParams{
		LandingPageID: page.ID,
		Name:          "Contact Us",
		Key:           "contact-us",
		SubmitLabel:   "Send Message",
	})
	if err != nil {
		t.Fatalf("CreateForm: %v", err)
	}

	// 2. Create Valid Field
	field, err := formService.CreateField(ctx, tenants.A.Scope, repository.CreateFormFieldParams{
		FormID:      form.ID,
		Key:         "email",
		Type:        domain.FormFieldTypeEmail,
		Label:       "Email Address",
		IsRequired:  true,
		SortOrder:   1,
	})
	if err != nil {
		t.Fatalf("CreateField (Valid): %v", err)
	}

	// 3. Create Invalid Field
	_, err = formService.CreateField(ctx, tenants.A.Scope, repository.CreateFormFieldParams{
		FormID:      form.ID,
		Key:         "invalid",
		Type:        domain.FormFieldType("invalid_type"),
		Label:       "Invalid",
		IsRequired:  false,
		SortOrder:   2,
	})
	if err == nil || err != service.ErrInvalidFieldType {
		t.Errorf("Expected ErrInvalidFieldType, got %v", err)
	}

	// 4. Get Form with Fields
	loadedForm, err := formService.GetForm(ctx, tenants.A.Scope, form.ID)
	if err != nil {
		t.Fatalf("GetForm: %v", err)
	}

	if len(loadedForm.Fields) != 1 || loadedForm.Fields[0].ID != field.ID {
		t.Errorf("Expected 1 field in loaded form, got %d", len(loadedForm.Fields))
	}
}
