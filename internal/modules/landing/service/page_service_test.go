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

func TestPageServiceIntegration(t *testing.T) {
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
	pageSvc := service.NewPageService(pageRepo, sectionRepo)

	// 1. Create Page
	page, err := pageSvc.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Promo Halaman Baru",
		Title:      "Promo",
		Slug:       testutil.UniqueCode("promo-halaman-baru"),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
		CreatedBy:  "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create page: %v", err)
	}

	if !strings.HasPrefix(page.Slug, "promo-halaman-baru") {
		t.Errorf("Expected slug to be sanitized and have prefix 'promo-halaman-baru', got: %s", page.Slug)
	}

	// 2. Add Section
	_, err = sectionRepo.Create(ctx, tenants.A.Scope, repository.CreateSectionParams{
		LandingPageID: page.ID,
		Key:           "hero-1",
		Type:          domain.SectionTypeHero,
		Name:          "Hero Section",
		SortOrder:     0,
		IsEnabled:     true,
		Content:       map[string]any{"title": "Welcome"},
		Style:         map[string]any{"color": "blue"},
		CreatedBy:     "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create section: %v", err)
	}

	// 3. Duplicate Page
	dupPage, err := pageSvc.Duplicate(ctx, tenants.A.Scope, service.DuplicatePageParams{
		PageID: page.ID,
		UserID: "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Duplicate page: %v", err)
	}

	if dupPage.Status != domain.PageStatusDraft {
		t.Errorf("Expected duplicated page to be draft, got: %s", dupPage.Status)
	}
	if !strings.Contains(dupPage.Slug, "-copy-") {
		t.Errorf("Expected slug to contain '-copy-', got: %s", dupPage.Slug)
	}

	// Verify sections copied
	dupSections, err := sectionRepo.ListByPage(ctx, tenants.A.Scope, dupPage.ID)
	if err != nil {
		t.Fatalf("List sections of duplicated page: %v", err)
	}
	if len(dupSections) != 1 {
		t.Fatalf("Expected 1 copied section, got %d", len(dupSections))
	}
	if dupSections[0].Name != "Hero Section" {
		t.Errorf("Expected section name 'Hero Section', got: %s", dupSections[0].Name)
	}

	// 4. Archive & Restore
	err = pageSvc.Archive(ctx, tenants.A.Scope, dupPage.ID, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("Archive page: %v", err)
	}
	pArchived, _ := pageSvc.Get(ctx, tenants.A.Scope, dupPage.ID)
	if pArchived.Status != domain.PageStatusArchived {
		t.Errorf("Expected status archived, got: %s", pArchived.Status)
	}

	err = pageSvc.Restore(ctx, tenants.A.Scope, dupPage.ID, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("Restore page: %v", err)
	}
	pRestored, _ := pageSvc.Get(ctx, tenants.A.Scope, dupPage.ID)
	if pRestored.Status != domain.PageStatusDraft {
		t.Errorf("Expected status draft after restore, got: %s", pRestored.Status)
	}
}
