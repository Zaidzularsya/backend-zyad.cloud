//go:build integration

package repository_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestMediaRepositoryIntegration(t *testing.T) {
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

	mediaRepo := repository.NewMediaRepository(db)

	w := 1920
	h := 1080

	assetA, err := mediaRepo.Create(ctx, tenants.A.Scope, repository.CreateMediaAssetParams{
		StorageKey:       "org-a/images/hero.jpg",
		Filename:         "hero.jpg",
		MimeType:         "image/jpeg",
		SizeBytes:        204800,
		Width:            &w,
		Height:           &h,
		AltText:          "Hero image",
		ProcessingStatus: domain.MediaProcessingPending,
		CreatedBy:        "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create media asset: %v", err)
	}

	_, err = mediaRepo.Get(ctx, tenants.B.Scope, assetA.ID)
	if err == nil {
		t.Error("Expected Tenant B to fail reading Tenant A's media asset")
	}

	err = mediaRepo.UpdateStatus(ctx, tenants.A.Scope, assetA.ID, domain.MediaProcessingCompleted)
	if err != nil {
		t.Fatalf("Update media status: %v", err)
	}

	updatedAsset, err := mediaRepo.Get(ctx, tenants.A.Scope, assetA.ID)
	if err != nil {
		t.Fatalf("Get updated media: %v", err)
	}
	if updatedAsset.ProcessingStatus != domain.MediaProcessingCompleted {
		t.Errorf("Expected completed status, got %s", updatedAsset.ProcessingStatus)
	}

	assets, err := mediaRepo.List(ctx, tenants.A.Scope)
	if err != nil {
		t.Fatalf("List media assets: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("Expected 1 media asset, got %d", len(assets))
	}

	err = mediaRepo.Delete(ctx, tenants.A.Scope, assetA.ID)
	if err != nil {
		t.Fatalf("Delete media asset: %v", err)
	}

	// Should not be listed anymore because of deleted_at
	assetsAfter, err := mediaRepo.List(ctx, tenants.A.Scope)
	if err != nil {
		t.Fatalf("List media assets after delete: %v", err)
	}
	if len(assetsAfter) != 0 {
		t.Fatalf("Expected 0 media assets, got %d", len(assetsAfter))
	}
}
