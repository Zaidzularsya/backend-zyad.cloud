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

func TestTemplateServiceIntegration(t *testing.T) {
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

	reusableRepo := repository.NewReusableRepository(db)
	sectionRepo := repository.NewSectionRepository(db)
	pageRepo := repository.NewPageRepository(db)
	
	templateService := service.NewTemplateService(reusableRepo, sectionRepo)

	// User ID for mock
	userID := "11111111-1111-1111-1111-111111111111"

	var tmpl domain.SectionTemplate

	t.Run("Create Template", func(t *testing.T) {
		params := repository.CreateSectionTemplateParams{
			Name:        "Test Hero Template",
			Description: "A reusable hero section",
			SectionType: domain.SectionTypeHero,
			Content:     map[string]any{"title": "Welcome"},
			Style:       map[string]any{"color": "red"},
			CreatedBy:   userID,
		}

		created, err := templateService.Create(ctx, tenants.A.Scope, params)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if created.ID == "" {
			t.Errorf("expected non-empty ID")
		}
		if created.Name != params.Name {
			t.Errorf("expected Name %s, got %s", params.Name, created.Name)
		}

		tmpl = created
	})

	t.Run("FindByID Template", func(t *testing.T) {
		found, err := templateService.FindByID(ctx, tenants.A.Scope, tmpl.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if found.ID != tmpl.ID {
			t.Errorf("expected ID %s, got %s", tmpl.ID, found.ID)
		}
	})

	t.Run("Update Template", func(t *testing.T) {
		newName := "Updated Hero Template"
		params := repository.UpdateSectionTemplateParams{
			Name:      &newName,
			UpdatedBy: userID,
		}

		updated, err := templateService.Update(ctx, tenants.A.Scope, tmpl.ID, params)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}

		if updated.Name != newName {
			t.Errorf("expected Name %s, got %s", newName, updated.Name)
		}
	})

	t.Run("List Templates", func(t *testing.T) {
		list, err := templateService.List(ctx, tenants.A.Scope, domain.SectionTypeHero)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) < 1 {
			t.Errorf("expected at least 1 Template, got %d", len(list))
		}
	})

	t.Run("InstantiateToPage", func(t *testing.T) {
		// create a dummy page first
		pageParams := repository.CreatePageParams{
			Name:       "Test Page for Template",
			Title:      "Title",
			Slug:       strings.ReplaceAll(testutil.UniqueCode("tmpl-page-"), ".", "-"),
			Type:       domain.PageTypeCampaign,
			Status:     domain.PageStatusDraft,
			Visibility: domain.PageVisibilityPrivate,
		}
		page, err := pageRepo.Create(ctx, tenants.A.Scope, pageParams)
		if err != nil {
			t.Fatalf("CreatePage: %v", err)
		}

		sectionKey := "hero-section-1"
		section, err := templateService.InstantiateToPage(ctx, tenants.A.Scope, tmpl.ID, page.ID, sectionKey, 1, userID)
		if err != nil {
			t.Fatalf("InstantiateToPage: %v", err)
		}

		if section.ID == "" {
			t.Errorf("expected non-empty section ID")
		}
		if section.Type != domain.SectionTypeHero {
			t.Errorf("expected section type Hero, got %s", section.Type)
		}
		if section.LandingPageID != page.ID {
			t.Errorf("expected page ID %s, got %s", page.ID, section.LandingPageID)
		}
	})

	t.Run("Delete Template", func(t *testing.T) {
		err := templateService.Delete(ctx, tenants.A.Scope, tmpl.ID, userID)
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}

		// Should not be found after deletion
		_, err = templateService.FindByID(ctx, tenants.A.Scope, tmpl.ID)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
	})
}
