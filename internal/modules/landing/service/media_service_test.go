//go:build integration

package service_test

import (
	"context"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestMediaServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	mediaRepo := repository.NewMediaRepository(db)
	mediaService := service.NewMediaService(mediaRepo)

	// 1. Test Valid Upload
	asset, err := mediaService.UploadAsset(ctx, tenants.A.Scope, repository.CreateMediaAssetParams{
		Filename:  "logo.png",
		MimeType:  "image/png",
		SizeBytes: 1024 * 500, // 500KB
		AltText:   "Company Logo",
		CreatedBy: "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("UploadAsset (Valid): %v", err)
	}

	if asset.StorageKey == "" || !strings.Contains(asset.StorageKey, "landing-media") {
		t.Errorf("Expected valid storage key, got %s", asset.StorageKey)
	}

	// 2. Test Invalid MIME
	_, err = mediaService.UploadAsset(ctx, tenants.A.Scope, repository.CreateMediaAssetParams{
		Filename:  "script.sh",
		MimeType:  "application/x-sh",
		SizeBytes: 1024,
	})
	if err == nil || err != service.ErrInvalidMimeType {
		t.Errorf("Expected ErrInvalidMimeType, got %v", err)
	}

	// 3. Test File Too Large
	_, err = mediaService.UploadAsset(ctx, tenants.A.Scope, repository.CreateMediaAssetParams{
		Filename:  "big.mp4",
		MimeType:  "video/mp4",
		SizeBytes: 1024 * 1024 * 50, // 50MB
	})
	if err == nil || err != service.ErrFileTooLarge {
		t.Errorf("Expected ErrFileTooLarge, got %v", err)
	}

	// 4. Test List & Delete
	assets, err := mediaService.ListAssets(ctx, tenants.A.Scope)
	if err != nil || len(assets) == 0 {
		t.Fatalf("ListAssets: %v", err)
	}

	err = mediaService.DeleteAsset(ctx, tenants.A.Scope, asset.ID)
	if err != nil {
		t.Fatalf("DeleteAsset: %v", err)
	}
}
