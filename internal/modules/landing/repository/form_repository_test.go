//go:build integration

package repository_test

import (
	"context"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestFormRepositoryLifecycleAndIsolationIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	_, err := db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES 
		('11111111-1111-1111-1111-111111111111', 'Mock User A', 'mock_a@example.com', 'active'),
		('22222222-2222-2222-2222-222222222222', 'Mock User B', 'mock_b@example.com', 'active')
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		t.Fatalf("failed to insert mock users: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES 
		($1, 'customer', 'organization-a', 'Organization A', 'active'),
		($2, 'customer', 'organization-b', 'Organization B', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID, tenants.B.OrganizationID)
	if err != nil {
		t.Fatalf("failed to insert mock organizations: %v", err)
	}

	pageRepo := repository.NewPageRepository(db)
	formRepo := repository.NewFormRepository(db)

	slugA := strings.ReplaceAll("page-"+testutil.UniqueCode("formrepoa"), ".", "-")
	slugB := strings.ReplaceAll("page-"+testutil.UniqueCode("formrepob"), ".", "-")

	// Create Pages
	pageA, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Page A",
		Title:      "Campaign A",
		Slug:       slugA,
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
		CreatedBy:  "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create page for Tenant A: %v", err)
	}
	_, err = pageRepo.Create(ctx, tenants.B.Scope, repository.CreatePageParams{
		Name:       "Page B",
		Title:      "Campaign B",
		Slug:       slugB,
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
		CreatedBy:  "22222222-2222-2222-2222-222222222222",
	})
	if err != nil {
		t.Fatalf("Create page for Tenant B: %v", err)
	}

	// 1. Create a form for Tenant A
	formA, err := formRepo.Create(ctx, tenants.A.Scope, repository.CreateFormParams{
		LandingPageID:  pageA.ID,
		Name:           "Contact Us",
		Key:            "contact-form-1",
		Description:    "Main contact form",
		SubmitLabel:    "Send Message",
		SuccessMessage: "Thanks!",
		RedirectURL:    "",
		IsActive:       true,
		CreatedBy:      "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create form in page A: %v", err)
	}

	// 2. Add fields to Form A
	field1, err := formRepo.CreateField(ctx, tenants.A.Scope, repository.CreateFormFieldParams{
		FormID:      formA.ID,
		Key:         "full_name",
		Type:        domain.FormFieldTypeText,
		Label:       "Full Name",
		Placeholder: "John Doe",
		IsRequired:  true,
		SortOrder:   0,
	})
	if err != nil {
		t.Fatalf("Create field 1: %v", err)
	}

	field2, err := formRepo.CreateField(ctx, tenants.A.Scope, repository.CreateFormFieldParams{
		FormID:      formA.ID,
		Key:         "email_addr",
		Type:        domain.FormFieldTypeEmail,
		Label:       "Email Address",
		Placeholder: "john@example.com",
		IsRequired:  true,
		SortOrder:   1,
	})
	if err != nil {
		t.Fatalf("Create field 2: %v", err)
	}

	// 3. Verify Isolation: Tenant B cannot read Form A
	_, err = formRepo.FindByID(ctx, tenants.B.Scope, formA.ID)
	if err == nil {
		t.Error("Expected Tenant B to fail reading Tenant A's form, got success")
	}

	// 4. Update Field
	newLabel := "Enter Email"
	updatedField, err := formRepo.UpdateField(ctx, tenants.A.Scope, field2.ID, repository.UpdateFormFieldParams{
		Label: &newLabel,
	})
	if err != nil {
		t.Fatalf("Update field 2: %v", err)
	}
	if updatedField.Label != "Enter Email" {
		t.Errorf("Expected label to be updated to 'Enter Email', got %s", updatedField.Label)
	}

	// 5. Reorder Fields
	err = formRepo.ReorderFields(ctx, tenants.A.Scope, formA.ID, []repository.FormFieldReorderParam{
		{ID: field1.ID, SortOrder: 1},
		{ID: field2.ID, SortOrder: 0},
	})
	if err != nil {
		t.Fatalf("Reorder fields: %v", err)
	}

	fields, err := formRepo.ListFieldsByForm(ctx, tenants.A.Scope, formA.ID)
	if err != nil {
		t.Fatalf("List fields: %v", err)
	}
	if len(fields) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(fields))
	}
	if fields[0].ID != field2.ID { // field2 should be first (SortOrder 0)
		t.Errorf("Expected field2 to be first, got %v", fields[0].ID)
	}

	// 6. Delete Field
	err = formRepo.DeleteField(ctx, tenants.A.Scope, field1.ID)
	if err != nil {
		t.Fatalf("Delete field 1: %v", err)
	}

	// 7. Delete Form (Soft Delete)
	err = formRepo.Delete(ctx, tenants.A.Scope, formA.ID)
	if err != nil {
		t.Fatalf("Delete form A: %v", err)
	}

	_, err = formRepo.FindByID(ctx, tenants.A.Scope, formA.ID)
	if err == nil {
		t.Error("Expected form A to be soft deleted, got success finding it")
	}
}
