package service

import (
	"context"
	"errors"
	"path"
	"strings"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/storage"
)

var (
	ErrInvalidMimeType = errors.New("invalid mime type")
	ErrFileTooLarge    = errors.New("file too large")
)

const MaxMediaSizeBytes = 10 * 1024 * 1024 // 10MB

type mediaService struct {
	mediaRepo repository.MediaRepository
}

func NewMediaService(mediaRepo repository.MediaRepository) MediaService {
	return &mediaService{
		mediaRepo: mediaRepo,
	}
}

func (s *mediaService) UploadAsset(ctx context.Context, scope coretenant.Scope, params repository.CreateMediaAssetParams) (domain.LandingMediaAsset, error) {
	// Validate Size
	if params.SizeBytes > MaxMediaSizeBytes {
		return domain.LandingMediaAsset{}, ErrFileTooLarge
	}

	// Validate MIME
	validMimes := map[string]bool{
		"image/jpeg":      true,
		"image/png":       true,
		"image/svg+xml":   true,
		"image/webp":      true,
		"application/pdf": true,
		"video/mp4":       true,
	}

	mimeLower := strings.ToLower(params.MimeType)
	if !validMimes[mimeLower] {
		return domain.LandingMediaAsset{}, ErrInvalidMimeType
	}

	// Generate Storage Key
	storageInput := storage.ObjectKeyInput{
		Scope:     scope,
		Class:     storage.ObjectClassPublic,
		Namespace: "landing-media",
		Filename:  params.Filename,
		Extension: path.Ext(params.Filename),
	}
	key, err := storage.NewRandomObjectKey(storageInput)
	if err != nil {
		return domain.LandingMediaAsset{}, err
	}

	params.StorageKey = key
	if params.ProcessingStatus == "" {
		params.ProcessingStatus = domain.MediaProcessingCompleted
	}

	return s.mediaRepo.Create(ctx, scope, params)
}

func (s *mediaService) GetAsset(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingMediaAsset, error) {
	return s.mediaRepo.Get(ctx, scope, id)
}

func (s *mediaService) ListAssets(ctx context.Context, scope coretenant.Scope) ([]domain.LandingMediaAsset, error) {
	return s.mediaRepo.List(ctx, scope)
}

func (s *mediaService) DeleteAsset(ctx context.Context, scope coretenant.Scope, id string) error {
	return s.mediaRepo.Delete(ctx, scope, id)
}

func (s *mediaService) MarkAssetProcessing(ctx context.Context, scope coretenant.Scope, id string, status domain.MediaProcessingStatus) error {
	return s.mediaRepo.UpdateStatus(ctx, scope, id, status)
}
