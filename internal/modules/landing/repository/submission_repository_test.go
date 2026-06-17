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

func TestSubmissionRepositoryLifecycleAndIsolationIntegration(t *testing.T) {
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
	subRepo := repository.NewSubmissionRepository(db)

	slugA := strings.ReplaceAll("page-"+testutil.UniqueCode("subrepoa"), ".", "-")

	// Create Page
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

	// Create Form
	formA, err := formRepo.Create(ctx, tenants.A.Scope, repository.CreateFormParams{
		LandingPageID: pageA.ID,
		Name:          "Contact Us",
		Key:           "contact-form-1",
		IsActive:      true,
		CreatedBy:     "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create form in page A: %v", err)
	}

	// 1. Create a submission
	subA, err := subRepo.Create(ctx, tenants.A.Scope, repository.CreateSubmissionParams{
		LandingPageID: pageA.ID,
		FormID:        formA.ID,
		Reference:     "SUB-12345",
		Status:        domain.SubmissionStatusNew,
		SubmittedData: map[string]any{"email": "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Create submission: %v", err)
	}

	// 2. Tenant B cannot read Tenant A's submission
	_, err = subRepo.FindByID(ctx, tenants.B.Scope, subA.ID)
	if err == nil {
		t.Error("Expected Tenant B to fail reading Tenant A's submission, got success")
	}

	// 3. Update Submission Status
	newStatus := domain.SubmissionStatusContacted
	updatedSub, err := subRepo.Update(ctx, tenants.A.Scope, subA.ID, repository.UpdateSubmissionParams{
		Status: &newStatus,
	})
	if err != nil {
		t.Fatalf("Update submission: %v", err)
	}
	if updatedSub.Status != domain.SubmissionStatusContacted {
		t.Errorf("Expected status 'contacted', got %v", updatedSub.Status)
	}

	// 4. Create Note
	err = subRepo.CreateNote(ctx, tenants.A.Scope, repository.CreateSubmissionNoteParams{
		SubmissionID: subA.ID,
		Note:         "Followed up via email",
		CreatedBy:    "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create submission note: %v", err)
	}

	// 5. Delete Submission
	err = subRepo.Delete(ctx, tenants.A.Scope, subA.ID)
	if err != nil {
		t.Fatalf("Delete submission: %v", err)
	}

	_, err = subRepo.FindByID(ctx, tenants.A.Scope, subA.ID)
	if err == nil {
		t.Error("Expected submission to be soft deleted, got success")
	}
}
