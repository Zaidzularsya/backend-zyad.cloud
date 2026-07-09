//go:build integration

package service_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestNavigationServiceIntegration(t *testing.T) {
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
	pageRepo := repository.NewPageRepository(db)
	navService := service.NewNavigationService(reusableRepo, pageRepo)

	userID := "11111111-1111-1111-1111-111111111111"

	var menu domain.LandingMenu
	var item1, item2, item3 domain.LandingMenuItem

	t.Run("Create Menu", func(t *testing.T) {
		params := repository.CreateMenuParams{
			Name:      "Header Menu",
			Location:  domain.MenuLocationHeader,
			IsActive:  true,
			CreatedBy: userID,
		}

		created, err := navService.CreateMenu(ctx, tenants.A.Scope, params)
		if err != nil {
			t.Fatalf("CreateMenu: %v", err)
		}
		if created.ID == "" {
			t.Errorf("expected non-empty ID")
		}
		menu = created
	})

	t.Run("Create Menu Items - Check Depth", func(t *testing.T) {
		// Depth 0 (Root)
		item1, err = navService.CreateMenuItem(ctx, tenants.A.Scope, repository.CreateMenuItemParams{
			MenuID:      menu.ID,
			Label:       "Root",
			LinkType:    domain.LinkTypeAnchor,
			Destination: "/",
			Target:      domain.CTATargetSelf,
			SortOrder:   1,
			IsEnabled:   true,
		})
		if err != nil {
			t.Fatalf("CreateMenuItem 1: %v", err)
		}

		// Depth 1
		item2, err = navService.CreateMenuItem(ctx, tenants.A.Scope, repository.CreateMenuItemParams{
			MenuID:      menu.ID,
			ParentID:    &item1.ID,
			Label:       "Child 1",
			LinkType:    domain.LinkTypeAnchor,
			Destination: "/child",
			Target:      domain.CTATargetSelf,
			SortOrder:   1,
			IsEnabled:   true,
		})
		if err != nil {
			t.Fatalf("CreateMenuItem 2: %v", err)
		}

		// Depth 2
		item3, err = navService.CreateMenuItem(ctx, tenants.A.Scope, repository.CreateMenuItemParams{
			MenuID:      menu.ID,
			ParentID:    &item2.ID,
			Label:       "Child 2",
			LinkType:    domain.LinkTypeAnchor,
			Destination: "/child/2",
			Target:      domain.CTATargetSelf,
			SortOrder:   1,
			IsEnabled:   true,
		})
		if err != nil {
			t.Fatalf("CreateMenuItem 3: %v", err)
		}

		// Depth 3 (Should fail)
		_, err = navService.CreateMenuItem(ctx, tenants.A.Scope, repository.CreateMenuItemParams{
			MenuID:      menu.ID,
			ParentID:    &item3.ID,
			Label:       "Child 3",
			LinkType:    domain.LinkTypeAnchor,
			Destination: "/child/3",
			Target:      domain.CTATargetSelf,
			SortOrder:   1,
			IsEnabled:   true,
		})
		if err != service.ErrMaxMenuDepth {
			t.Errorf("expected ErrMaxMenuDepth, got %v", err)
		}
	})

	t.Run("Update Menu Item - Depth Check", func(t *testing.T) {
		// Cannot move Item 2 to be a child of Item 3 because Item 3 is already a child of Item 2 (cycle simulation via depth)
		// Wait, a true cycle check might be needed, but depth check will also trigger if depth gets too long.
		// Let's create an independent Item 4 and try to attach it to Item 3
		item4, err := navService.CreateMenuItem(ctx, tenants.A.Scope, repository.CreateMenuItemParams{
			MenuID:      menu.ID,
			Label:       "Independent",
			LinkType:    domain.LinkTypeAnchor,
			Destination: "/ind",
			Target:      domain.CTATargetSelf,
			SortOrder:   2,
			IsEnabled:   true,
		})
		if err != nil {
			t.Fatalf("CreateMenuItem 4: %v", err)
		}

		// Move Item 4 to be child of Item 3 (depth 3)
		_, err = navService.UpdateMenuItem(ctx, tenants.A.Scope, item4.ID, repository.UpdateMenuItemParams{
			ParentID: &item3.ID,
		})
		if err != service.ErrMaxMenuDepth {
			t.Errorf("expected ErrMaxMenuDepth when updating, got %v", err)
		}
	})

	t.Run("Delete Menu Item", func(t *testing.T) {
		err := navService.DeleteMenuItem(ctx, tenants.A.Scope, item1.ID)
		if err != nil {
			t.Fatalf("DeleteMenuItem: %v", err)
		}
	})

	t.Run("Delete Menu", func(t *testing.T) {
		err := navService.DeleteMenu(ctx, tenants.A.Scope, menu.ID, userID)
		if err != nil {
			t.Fatalf("DeleteMenu: %v", err)
		}
	})
}
