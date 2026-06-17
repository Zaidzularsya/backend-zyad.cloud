//go:build integration

package repository_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestReusableRepositoryIntegration(t *testing.T) {
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

	reusableRepo := repository.NewReusableRepository(db)

	// --- 1. Section Templates ---
	tmplA, err := reusableRepo.CreateSectionTemplate(ctx, tenants.A.Scope, repository.CreateSectionTemplateParams{
		Name:        "Hero Standard",
		Description: "A standard hero",
		SectionType: domain.SectionTypeHero,
		Content:     map[string]any{"headline": "Welcome"},
		Style:       map[string]any{"bg": "white"},
		CreatedBy:   "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create section template: %v", err)
	}

	_, err = reusableRepo.GetSectionTemplate(ctx, tenants.B.Scope, tmplA.ID)
	if err == nil {
		t.Error("Expected Tenant B to fail reading Tenant A's section template")
	}

	templates, err := reusableRepo.ListSectionTemplates(ctx, tenants.A.Scope, domain.SectionTypeHero)
	if err != nil {
		t.Fatalf("List section templates: %v", err)
	}
	if len(templates) != 1 {
		t.Errorf("Expected 1 section template, got %d", len(templates))
	}

	err = reusableRepo.DeleteSectionTemplate(ctx, tenants.A.Scope, tmplA.ID, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("Delete section template: %v", err)
	}

	// --- 2. CTAs ---
	ctaA, err := reusableRepo.CreateCTA(ctx, tenants.A.Scope, repository.CreateCTAParams{
		Name:        "Contact Us CTA",
		Label:       "Contact Us",
		Type:        domain.CTATypeInternalPage,
		Target:      domain.CTATargetSelf,
		Destination: "/contact",
		TrackingKey: "cta-contact-us",
		CreatedBy:   "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create CTA: %v", err)
	}

	err = reusableRepo.DeleteCTA(ctx, tenants.A.Scope, ctaA.ID, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("Delete CTA: %v", err)
	}

	// --- 3. Menus and Menu Items ---
	menuA, err := reusableRepo.CreateMenu(ctx, tenants.A.Scope, repository.CreateMenuParams{
		Name:      "Header Menu",
		Location:  domain.MenuLocationHeader,
		IsActive:  true,
		CreatedBy: "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create menu: %v", err)
	}

	item1, err := reusableRepo.CreateMenuItem(ctx, tenants.A.Scope, repository.CreateMenuItemParams{
		MenuID:      menuA.ID,
		Label:       "Home",
		LinkType:    domain.LinkTypeInternalPage,
		Destination: "/",
		Target:      domain.CTATargetSelf,
		SortOrder:   0,
		IsEnabled:   true,
	})
	if err != nil {
		t.Fatalf("Create menu item 1: %v", err)
	}

	item2, err := reusableRepo.CreateMenuItem(ctx, tenants.A.Scope, repository.CreateMenuItemParams{
		MenuID:      menuA.ID,
		Label:       "About",
		LinkType:    domain.LinkTypeInternalPage,
		Destination: "/about",
		Target:      domain.CTATargetSelf,
		SortOrder:   1,
		IsEnabled:   true,
	})
	if err != nil {
		t.Fatalf("Create menu item 2: %v", err)
	}

	// Reorder
	err = reusableRepo.ReorderMenuItems(ctx, tenants.A.Scope, menuA.ID, []string{item2.ID, item1.ID})
	if err != nil {
		t.Fatalf("Reorder menu items: %v", err)
	}

	items, err := reusableRepo.ListMenuItems(ctx, tenants.A.Scope, menuA.ID)
	if err != nil {
		t.Fatalf("List menu items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Expected 2 menu items, got %d", len(items))
	}
	if items[0].ID != item2.ID || items[1].ID != item1.ID {
		t.Errorf("Reorder failed, got %s then %s", items[0].ID, items[1].ID)
	}

	err = reusableRepo.DeleteMenu(ctx, tenants.A.Scope, menuA.ID, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("Delete menu: %v", err)
	}
}
