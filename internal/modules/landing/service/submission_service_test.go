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

func TestSubmissionServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	pageRepo := repository.NewPageRepository(db)
	formRepo := repository.NewFormRepository(db)
	submissionRepo := repository.NewSubmissionRepository(db)

	submissionService := service.NewSubmissionService(submissionRepo, formRepo)

	// Create Page
	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Sub Page",
		Title:      "Sub Title",
		Slug:       strings.ReplaceAll(testutil.UniqueCode("sub-page-"), ".", "-"),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
	})
	if err != nil {
		t.Fatalf("Create page: %v", err)
	}

	// Create Form
	form, err := formRepo.Create(ctx, tenants.A.Scope, repository.CreateFormParams{
		LandingPageID: page.ID,
		Name:          "Contact Us",
		Key:           "contact-us",
		IsActive:      true,
	})
	if err != nil {
		t.Fatalf("Create form: %v", err)
	}

	// 1. Submit Valid Form
	submission, err := submissionService.SubmitForm(ctx, tenants.A.Scope, repository.CreateSubmissionParams{
		LandingPageID: page.ID,
		FormID:        form.ID,
		SubmittedData: map[string]any{"email": "test@example.com"},
	})
	if err != nil {
		t.Fatalf("SubmitForm (Valid): %v", err)
	}

	if submission.Status != domain.SubmissionStatusNew {
		t.Errorf("Expected status to be New, got %v", submission.Status)
	}
	if submission.IdempotencyKey == "" {
		t.Errorf("Expected idempotency key to be generated")
	}

	// 2. Submit SPAM
	_, err = submissionService.SubmitForm(ctx, tenants.A.Scope, repository.CreateSubmissionParams{
		LandingPageID: page.ID,
		FormID:        form.ID,
		SubmittedData: map[string]any{"email": "spam@example.com", "_honey": "bot"},
	})
	if err == nil || err != service.ErrSpamDetected {
		t.Errorf("Expected ErrSpamDetected, got %v", err)
	}

	// 3. Update Status
	updated, err := submissionService.UpdateSubmissionStatus(ctx, tenants.A.Scope, submission.ID, domain.SubmissionStatusContacted)
	if err != nil {
		t.Fatalf("UpdateSubmissionStatus: %v", err)
	}
	if updated.Status != domain.SubmissionStatusContacted {
		t.Errorf("Expected status to be Contacted, got %v", updated.Status)
	}

	// 4. Inactive Form
	_, _ = formRepo.Update(ctx, tenants.A.Scope, form.ID, repository.UpdateFormParams{
		IsActive: new(bool), // false
	})
	_, err = submissionService.SubmitForm(ctx, tenants.A.Scope, repository.CreateSubmissionParams{
		LandingPageID: page.ID,
		FormID:        form.ID,
		SubmittedData: map[string]any{"email": "test@example.com"},
	})
	if err == nil || err != service.ErrFormNotActive {
		t.Errorf("Expected ErrFormNotActive, got %v", err)
	}
}
