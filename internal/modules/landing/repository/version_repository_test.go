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

func TestVersionRepositoryIntegration(t *testing.T) {
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
	versionRepo := repository.NewVersionRepository(db)

	slugA := strings.ReplaceAll("page-"+testutil.UniqueCode("versionrepoa"), ".", "-")

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

	// 1. Create a version
	verA, err := versionRepo.Create(ctx, tenants.A.Scope, repository.CreateVersionParams{
		LandingPageID: pageA.ID,
		Version:       1,
		ChangeNote:    "Initial release",
		Snapshot:      map[string]any{"key": "value"},
		CreatedBy:     "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create version: %v", err)
	}

	// 2. Tenant B cannot read Tenant A's version
	_, err = versionRepo.FindByID(ctx, tenants.B.Scope, verA.ID)
	if err == nil {
		t.Error("Expected Tenant B to fail reading Tenant A's version, got success")
	}

	// 3. Create Redirect
	redirectA, err := versionRepo.CreateRedirect(ctx, tenants.A.Scope, repository.CreateSlugRedirectParams{
		SourceSlug: slugA + "-old",
		TargetSlug: slugA,
	})
	if err != nil {
		t.Fatalf("Create redirect: %v", err)
	}

	// 4. Update Redirect
	_, err = versionRepo.UpdateRedirect(ctx, tenants.A.Scope, redirectA.ID, slugA+"-new")
	if err != nil {
		t.Fatalf("Update redirect: %v", err)
	}

	// 5. Delete Redirect
	err = versionRepo.DeleteRedirect(ctx, tenants.A.Scope, redirectA.ID)
	if err != nil {
		t.Fatalf("Delete redirect: %v", err)
	}
}

func TestBrandingRepositoryIntegration(t *testing.T) {
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

	brandingRepo := repository.NewBrandingRepository(db)

	companyName := "Org A Company"

	// 1. Upsert Default Branding
	brandA, err := brandingRepo.Upsert(ctx, tenants.A.Scope, repository.CreateBrandingParams{
		CompanyName: &companyName,
	})
	if err != nil {
		t.Fatalf("Upsert default branding: %v", err)
	}

	// 2. Upsert it again to test CONFLICT DO UPDATE
	updatedCompanyName := "Org A Ltd"
	brandA, err = brandingRepo.Upsert(ctx, tenants.A.Scope, repository.CreateBrandingParams{
		CompanyName: &updatedCompanyName,
	})
	if err != nil {
		t.Fatalf("Upsert default branding update: %v", err)
	}
	if brandA.CompanyName != updatedCompanyName {
		t.Fatalf("Expected %s, got %s", updatedCompanyName, brandA.CompanyName)
	}

	// 3. Tenant B cannot read Tenant A's branding
	_, err = brandingRepo.GetDefault(ctx, tenants.B.Scope)
	if err == nil {
		t.Error("Expected Tenant B to fail reading Tenant A's branding, got success")
	}

	pageRepo := repository.NewPageRepository(db)
	slugA := strings.ReplaceAll("page-"+testutil.UniqueCode("brandingrepoa"), ".", "-")

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

	// 4. Upsert Page Override Branding
	pageBrandName := "Campaign Brand"
	pageBrandA, err := brandingRepo.Upsert(ctx, tenants.A.Scope, repository.CreateBrandingParams{
		LandingPageID: &pageA.ID,
		CompanyName:   &pageBrandName,
	})
	if err != nil {
		t.Fatalf("Upsert page branding: %v", err)
	}

	// 5. Upsert page branding again to test CONFLICT
	updatedPageBrandName := "Campaign Brand V2"
	pageBrandA, err = brandingRepo.Upsert(ctx, tenants.A.Scope, repository.CreateBrandingParams{
		LandingPageID: &pageA.ID,
		CompanyName:   &updatedPageBrandName,
	})
	if err != nil {
		t.Fatalf("Upsert page branding update: %v", err)
	}
	if pageBrandA.CompanyName != updatedPageBrandName {
		t.Fatalf("Expected %s, got %s", updatedPageBrandName, pageBrandA.CompanyName)
	}
}
