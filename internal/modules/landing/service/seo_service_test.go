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

func TestSeoServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	// Users and Org already inserted in other tests if they run sequentially,
	// but DO NOTHING on conflict so we are safe to insert again.
	_, _ = db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES ('11111111-1111-1111-1111-111111111111', 'Mock User A', 'mock_a@example.com', 'active')
		ON CONFLICT DO NOTHING
	`)
	_, _ = db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'organization-a', 'Organization A', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID)

	pageRepo := repository.NewPageRepository(db)
	seoService := service.NewSeoService(pageRepo)

	// Create Page
	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "SEO Page",
		Title:      "SEO Title",
		Slug:       strings.ReplaceAll(testutil.UniqueCode("seo-page"), ".", "-"),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		CreatedBy:  "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create page: %v", err)
	}

	// Test Update with Invalid URL
	_, err = seoService.UpdatePageSEO(ctx, tenants.A.Scope, page.ID, map[string]any{
		"canonical_url": "invalid-url",
	})
	if err == nil {
		t.Error("Expected error for invalid canonical_url, got nil")
	}

	// Test Update with Valid URL and Data
	_, err = seoService.UpdatePageSEO(ctx, tenants.A.Scope, page.ID, map[string]any{
		"canonical_url": "https://example.com/seo-page",
		"keywords":      "seo, test",
	})
	if err != nil {
		t.Fatalf("UpdatePageSEO: %v", err)
	}

	// Test GetEffectiveSEO
	effectiveSEO, err := seoService.GetEffectiveSEO(ctx, tenants.A.Scope, page.ID)
	if err != nil {
		t.Fatalf("GetEffectiveSEO: %v", err)
	}

	if effectiveSEO["canonical_url"] != "https://example.com/seo-page" {
		t.Errorf("Expected canonical_url to be 'https://example.com/seo-page', got: %v", effectiveSEO["canonical_url"])
	}
	
	// Check Fallback Metadata
	if effectiveSEO["title"] != "SEO Title" {
		t.Errorf("Expected fallback title to be 'SEO Title', got: %v", effectiveSEO["title"])
	}
	if effectiveSEO["description"] != "SEO Page - SEO Title" {
		t.Errorf("Expected fallback description to be 'SEO Page - SEO Title', got: %v", effectiveSEO["description"])
	}
	if effectiveSEO["robots"] != "index, follow" {
		t.Errorf("Expected fallback robots to be 'index, follow', got: %v", effectiveSEO["robots"])
	}
}
