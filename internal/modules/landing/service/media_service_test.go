//go:build integration

package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/platform/database/testutil"
	"zyad.cloud/internal/platform/storage"
)

func TestMediaServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	storageRoot := t.TempDir()
	provider, err := storage.NewLocalProvider(storageRoot)
	if err != nil {
		t.Fatalf("NewLocalProvider: %v", err)
	}

	mediaRepo := repository.NewMediaRepository(db)
	mediaService := service.NewMediaService(
		mediaRepo,
		service.WithMediaObjectStorage(provider),
	)

	// 1. Test Valid Upload
	content := strings.Repeat("x", 1024*500)
	asset, err := mediaService.UploadAsset(ctx, tenants.A.Scope, repository.CreateMediaAssetParams{
		Filename:  "logo.png",
		MimeType:  "image/png",
		SizeBytes: int64(len(content)),
		AltText:   "Company Logo",
		CreatedBy: "11111111-1111-1111-1111-111111111111",
	}, strings.NewReader(content))
	if err != nil {
		t.Fatalf("UploadAsset (Valid): %v", err)
	}

	if asset.StorageKey == "" || !strings.Contains(asset.StorageKey, "landing-media") {
		t.Errorf("Expected valid storage key, got %s", asset.StorageKey)
	}
	if asset.PublicURL != "/public/media/"+asset.StorageKey {
		t.Errorf("Expected public URL derived from storage key, got %s", asset.PublicURL)
	}

	storedPath := filepath.Join(storageRoot, filepath.FromSlash(asset.StorageKey))
	stored, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatalf("uploaded file should exist on disk: %v", err)
	}
	if len(stored) != len(content) {
		t.Errorf("stored file size = %d, want %d", len(stored), len(content))
	}
	if _, err := provider.ResolvePublic(asset.StorageKey); err != nil {
		t.Errorf("ResolvePublic: %v", err)
	}

	// 2. Test Invalid MIME
	_, err = mediaService.UploadAsset(ctx, tenants.A.Scope, repository.CreateMediaAssetParams{
		Filename:  "script.sh",
		MimeType:  "application/x-sh",
		SizeBytes: 1024,
	}, strings.NewReader("payload"))
	if err == nil || err != service.ErrInvalidMimeType {
		t.Errorf("Expected ErrInvalidMimeType, got %v", err)
	}

	// 3. Test File Too Large
	_, err = mediaService.UploadAsset(ctx, tenants.A.Scope, repository.CreateMediaAssetParams{
		Filename:  "big.mp4",
		MimeType:  "video/mp4",
		SizeBytes: 1024 * 1024 * 50, // 50MB
	}, strings.NewReader("payload"))
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
	if _, err := os.Stat(storedPath); !os.IsNotExist(err) {
		t.Errorf("stored file should be removed after DeleteAsset, stat err = %v", err)
	}
}
