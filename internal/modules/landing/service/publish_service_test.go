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

func TestPublishServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	_, err := db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES 
		('11111111-1111-1111-1111-111111111111', 'Mock User A', 'mock_a@example.com', 'active')
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		t.Fatalf("failed to insert mock users: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES 
		($1, 'customer', 'organization-a', 'Organization A', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID)
	if err != nil {
		t.Fatalf("failed to insert mock organizations: %v", err)
	}

	pageRepo := repository.NewPageRepository(db)
	sectionRepo := repository.NewSectionRepository(db)
	formRepo := repository.NewFormRepository(db)
	brandingRepo := repository.NewBrandingRepository(db)
	versionRepo := repository.NewVersionRepository(db)
	documentRepo := repository.NewDocumentRepository(db)

	publishService := service.NewPublishService(db, pageRepo, sectionRepo, formRepo, brandingRepo, versionRepo, documentRepo, "test-secret")

	userID := "11111111-1111-1111-1111-111111111111"

	// Pre-requisites: Page
	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Publish Test Page",
		Title:      "My Title",
		Slug:       strings.ReplaceAll(testutil.UniqueCode("pub-page-"), ".", "-"),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPrivate,
		CreatedBy:  userID,
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	t.Run("ValidateForPublish - Without Sections and Branding", func(t *testing.T) {
		checklist, err := publishService.ValidateForPublish(ctx, tenants.A.Scope, page.ID)
		if err != nil {
			t.Fatalf("ValidateForPublish: %v", err)
		}
		if !checklist.IsValid {
			t.Errorf("Expected valid but got invalid: %v", checklist.Errors)
		}
		if len(checklist.Warnings) != 1 { // Missing sections
			t.Errorf("Expected 1 warning, got %d: %v", len(checklist.Warnings), checklist.Warnings)
		}
	})

	t.Run("Publish", func(t *testing.T) {
		version, err := publishService.Publish(ctx, tenants.A.Scope, page.ID, "Initial Release", userID)
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}
		if version.Version != 1 {
			t.Errorf("Expected version 1, got %d", version.Version)
		}

		// Verify page status
		updatedPage, err := pageRepo.FindByID(ctx, tenants.A.Scope, page.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if updatedPage.Status != domain.PageStatusPublished {
			t.Errorf("Expected Published status, got %s", updatedPage.Status)
		}
	})

	t.Run("Unpublish", func(t *testing.T) {
		err := publishService.Unpublish(ctx, tenants.A.Scope, page.ID, userID)
		if err != nil {
			t.Fatalf("Unpublish: %v", err)
		}

		updatedPage, err := pageRepo.FindByID(ctx, tenants.A.Scope, page.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if updatedPage.Status != domain.PageStatusDraft {
			t.Errorf("Expected Draft status, got %s", updatedPage.Status)
		}
	})

	var validToken string
	t.Run("Generate Preview Token", func(t *testing.T) {
		token, err := publishService.GeneratePreviewToken(ctx, tenants.A.Scope, page.ID, 60)
		if err != nil {
			t.Fatalf("GeneratePreviewToken: %v", err)
		}
		if token == "" {
			t.Errorf("Expected non-empty token")
		}
		validToken = token
	})

	t.Run("Validate Preview Token - Valid", func(t *testing.T) {
		pageID, err := publishService.ValidatePreviewToken(ctx, validToken)
		if err != nil {
			t.Fatalf("ValidatePreviewToken: %v", err)
		}
		if pageID != page.ID {
			t.Errorf("Expected page ID %s, got %s", page.ID, pageID)
		}
	})

	t.Run("Validate Preview Token - Invalid", func(t *testing.T) {
		_, err := publishService.ValidatePreviewToken(ctx, "invalid-token")
		if err == nil {
			t.Errorf("Expected error for invalid token")
		}
	})

	t.Run("Validate Preview Token - Expired", func(t *testing.T) {
		expiredToken, _ := publishService.GeneratePreviewToken(ctx, tenants.A.Scope, page.ID, -10)
		_, err := publishService.ValidatePreviewToken(ctx, expiredToken)
		if err != service.ErrInvalidToken {
			t.Errorf("Expected ErrInvalidToken, got %v", err)
		}
	})
}

func TestPublishServiceGrapesJSIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	_, _ = db.Exec(ctx, `INSERT INTO users (id, name, email, status)
		VALUES ('11111111-1111-1111-1111-111111111111','Mock User A','mock_a@example.com','active')
		ON CONFLICT DO NOTHING`)
	_, _ = db.Exec(ctx, `INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1,'customer','organization-a','Organization A','active') ON CONFLICT DO NOTHING`, tenants.A.OrganizationID)

	pageRepo := repository.NewPageRepository(db)
	sectionRepo := repository.NewSectionRepository(db)
	formRepo := repository.NewFormRepository(db)
	brandingRepo := repository.NewBrandingRepository(db)
	versionRepo := repository.NewVersionRepository(db)
	documentRepo := repository.NewDocumentRepository(db)
	publishService := service.NewPublishService(db, pageRepo, sectionRepo, formRepo, brandingRepo, versionRepo, documentRepo, "test-secret")
	userID := "11111111-1111-1111-1111-111111111111"

	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "GrapesJS Page",
		Title:      "GJS Title",
		Slug:       strings.ReplaceAll(testutil.UniqueCode("gjs-pub-"), ".", "-"),
		Type:       domain.PageTypeCampaign,
		Builder:    domain.PageBuilderGrapesJS,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		CreatedBy:  userID,
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	// No document yet → publish must fail validation.
	if checklist, _ := publishService.ValidateForPublish(ctx, tenants.A.Scope, page.ID); checklist.IsValid {
		t.Fatalf("expected invalid checklist for a grapesjs page with no document")
	}

	if _, err := documentRepo.Upsert(ctx, tenants.A.Scope, repository.UpsertDocumentParams{
		LandingPageID: page.ID,
		Project:       map[string]any{"pages": []any{}},
		HTML:          `<section><script>alert(1)</script><h1>Halo</h1></section>`,
		CSS:           `@import url(x); h1{color:red}`,
		UpdatedBy:     userID,
	}); err != nil {
		t.Fatalf("Upsert document: %v", err)
	}

	version, err := publishService.Publish(ctx, tenants.A.Scope, page.ID, "gjs release", userID)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if version.Snapshot["builder"] != string(domain.PageBuilderGrapesJS) {
		t.Fatalf("snapshot builder = %v", version.Snapshot["builder"])
	}
	html, _ := version.Snapshot["html"].(string)
	if strings.Contains(html, "<script") || !strings.Contains(html, "<h1>Halo</h1>") {
		t.Fatalf("snapshot html not sanitized: %q", html)
	}
	if css, _ := version.Snapshot["css"].(string); strings.Contains(strings.ToLower(css), "@import") {
		t.Fatalf("snapshot css not sanitized: %q", css)
	}
}
