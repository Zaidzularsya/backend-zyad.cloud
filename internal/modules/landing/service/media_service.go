package service

import (
	"context"
	"errors"
	"io"
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
	ErrStorageRequired = errors.New("media storage is not configured")
)

const MaxMediaSizeBytes = 10 * 1024 * 1024 // 10MB

const mediaPublicPathPrefix = "/public/media/"

type mediaService struct {
	mediaRepo     repository.MediaRepository
	objectStorage storage.ObjectStorage
	publicBaseURL string
}

type MediaServiceOption func(*mediaService)

func WithMediaObjectStorage(objectStorage storage.ObjectStorage) MediaServiceOption {
	return func(s *mediaService) {
		s.objectStorage = objectStorage
	}
}

// WithMediaPublicBaseURL menentukan origin backend untuk membangun public_url
// (mis. https://api.zyad.cloud). Kosong = path relatif.
func WithMediaPublicBaseURL(baseURL string) MediaServiceOption {
	return func(s *mediaService) {
		s.publicBaseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	}
}

func NewMediaService(
	mediaRepo repository.MediaRepository,
	options ...MediaServiceOption,
) MediaService {
	service := &mediaService{
		mediaRepo: mediaRepo,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *mediaService) UploadAsset(
	ctx context.Context,
	scope coretenant.Scope,
	params repository.CreateMediaAssetParams,
	content io.Reader,
) (domain.LandingMediaAsset, error) {
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

	if s.objectStorage == nil {
		return domain.LandingMediaAsset{}, ErrStorageRequired
	}
	if content == nil {
		return domain.LandingMediaAsset{}, errors.New("media content is required")
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

	// Batasi bacaan sesuai ukuran yang dilaporkan supaya client tidak bisa
	// menulis lebih besar dari validasi di atas.
	limited := io.LimitReader(content, MaxMediaSizeBytes+1)
	if err := s.objectStorage.Put(ctx, key, limited, mimeLower, params.SizeBytes); err != nil {
		return domain.LandingMediaAsset{}, err
	}

	params.StorageKey = key
	if params.ProcessingStatus == "" {
		params.ProcessingStatus = domain.MediaProcessingCompleted
	}

	asset, err := s.mediaRepo.Create(ctx, scope, params)
	if err != nil {
		// Row gagal dibuat — bersihkan file agar tidak yatim.
		_ = s.objectStorage.Delete(ctx, key)
		return domain.LandingMediaAsset{}, err
	}
	return s.withPublicURL(asset), nil
}

func (s *mediaService) GetAsset(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingMediaAsset, error) {
	asset, err := s.mediaRepo.Get(ctx, scope, id)
	if err != nil {
		return domain.LandingMediaAsset{}, err
	}
	return s.withPublicURL(asset), nil
}

func (s *mediaService) ListAssets(ctx context.Context, scope coretenant.Scope) ([]domain.LandingMediaAsset, error) {
	assets, err := s.mediaRepo.List(ctx, scope)
	if err != nil {
		return nil, err
	}
	for i := range assets {
		assets[i] = s.withPublicURL(assets[i])
	}
	return assets, nil
}

func (s *mediaService) DeleteAsset(ctx context.Context, scope coretenant.Scope, id string) error {
	asset, err := s.mediaRepo.Get(ctx, scope, id)
	if err != nil {
		return err
	}
	if err := s.mediaRepo.Delete(ctx, scope, id); err != nil {
		return err
	}
	if s.objectStorage != nil && asset.StorageKey != "" {
		// Best-effort: row sudah terhapus, file yatim tidak fatal.
		_ = s.objectStorage.Delete(ctx, asset.StorageKey)
	}
	return nil
}

func (s *mediaService) MarkAssetProcessing(ctx context.Context, scope coretenant.Scope, id string, status domain.MediaProcessingStatus) error {
	return s.mediaRepo.UpdateStatus(ctx, scope, id, status)
}

func (s *mediaService) withPublicURL(asset domain.LandingMediaAsset) domain.LandingMediaAsset {
	if asset.StorageKey == "" {
		return asset
	}
	asset.PublicURL = s.publicBaseURL + mediaPublicPathPrefix + asset.StorageKey
	return asset
}
