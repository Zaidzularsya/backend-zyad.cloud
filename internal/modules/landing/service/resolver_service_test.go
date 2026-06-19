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

func TestResolverServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	userID := "22222222-2222-2222-2222-222222222222"
	_, err := db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES ($1, 'Mock User B', 'mock_b@example.com', 'active')
		ON CONFLICT DO NOTHING
	`, userID)
	if err != nil {
		t.Fatalf("failed to insert mock user: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'organization-b', 'Organization B', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID)
	if err != nil {
		t.Fatalf("failed to insert mock org: %v", err)
	}

	pageRepo := repository.NewPageRepository(db)
	sectionRepo := repository.NewSectionRepository(db)
	formRepo := repository.NewFormRepository(db)
	brandingRepo := repository.NewBrandingRepository(db)
	versionRepo := repository.NewVersionRepository(db)
	resolverRepo := repository.NewResolverRepository(db)

	publishService := service.NewPublishService(db, pageRepo, sectionRepo, formRepo, brandingRepo, versionRepo, "test-secret")
	resolverService := service.NewResolverService(db, resolverRepo, versionRepo, pageRepo, sectionRepo, formRepo, brandingRepo, publishService)

	slug := strings.ReplaceAll(testutil.UniqueCode("resolve-page-"), ".", "-")
	
	// Create Page
	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Resolve Test Page",
		Title:      "Resolve Title",
		Slug:       slug,
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		CreatedBy:  userID,
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	t.Run("ResolveBySlug - Draft without Token", func(t *testing.T) {
		_, err := resolverService.ResolveBySlug(ctx, tenants.A.Scope, slug, "")
		if err != service.ErrPageNotPublished {
			t.Errorf("Expected ErrPageNotPublished, got %v", err)
		}
	})

	t.Run("ResolveBySlug - Draft with Valid Token", func(t *testing.T) {
		token, _ := publishService.GeneratePreviewToken(ctx, tenants.A.Scope, page.ID, 60)
		
		resolved, err := resolverService.ResolveBySlug(ctx, tenants.A.Scope, slug, token)
		if err != nil {
			t.Fatalf("ResolveBySlug: %v", err)
		}
		if resolved.Page.ID != page.ID {
			t.Errorf("Expected page ID %s, got %s", page.ID, resolved.Page.ID)
		}
		if !resolved.IsDraft {
			t.Errorf("Expected IsDraft to be true")
		}
	})

	t.Run("ResolveBySlug - Published", func(t *testing.T) {
		// Publish the page first
		_, err := publishService.Publish(ctx, tenants.A.Scope, page.ID, "Initial Release", userID)
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}

		resolved, err := resolverService.ResolveBySlug(ctx, tenants.A.Scope, slug, "")
		if err != nil {
			t.Fatalf("ResolveBySlug: %v", err)
		}
		if resolved.Page.ID != page.ID {
			t.Errorf("Expected page ID %s, got %s", page.ID, resolved.Page.ID)
		}
		if resolved.IsDraft {
			t.Errorf("Expected IsDraft to be false")
		}
		if resolved.Snapshot == nil {
			t.Errorf("Expected snapshot to be populated")
		}
	})

	t.Run("ResolveBySlug - Not Found", func(t *testing.T) {
		_, err := resolverService.ResolveBySlug(ctx, tenants.A.Scope, "non-existent-slug", "")
		if err != service.ErrPageNotFound {
			t.Errorf("Expected ErrPageNotFound, got %v", err)
		}
	})
}
