package service

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/storage"
)

// ErrUploadNotFound dikembalikan saat konfirmasi upload dipanggil tapi object
// belum benar-benar ada di storage.
var ErrUploadNotFound = errors.New("uploaded object not found in storage")

type PresignUploadParams struct {
	Filename  string
	MimeType  string
	SizeBytes int64
}

type PresignUploadResult struct {
	ObjectKey string            `json:"object_key"`
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type ConfirmUploadParams struct {
	ObjectKey string
	Filename  string
	MimeType  string
	SizeBytes int64
	AltText   string
	CreatedBy string
}

type PresignDownloadResult struct {
	ObjectKey   string    `json:"object_key"`
	DownloadURL string    `json:"download_url"`
	Method      string    `json:"method"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s *mediaService) presigner() (storage.PresignedStorage, bool) {
	presigner, ok := s.objectStorage.(storage.PresignedStorage)
	return presigner, ok
}

// PresignUpload membuat object key baru dan mengembalikan URL bertanda tangan
// agar client mengunggah file langsung ke object storage.
func (s *mediaService) PresignUpload(
	ctx context.Context,
	scope coretenant.Scope,
	params PresignUploadParams,
) (PresignUploadResult, error) {
	if s.objectStorage == nil {
		return PresignUploadResult{}, ErrStorageRequired
	}
	presigner, ok := s.presigner()
	if !ok {
		return PresignUploadResult{}, storage.ErrPresignUnsupported
	}
	if params.SizeBytes > MaxMediaSizeBytes {
		return PresignUploadResult{}, ErrFileTooLarge
	}
	mimeLower := strings.ToLower(params.MimeType)
	if !allowedMediaMimeTypes[mimeLower] {
		return PresignUploadResult{}, ErrInvalidMimeType
	}

	key, err := storage.NewRandomObjectKey(storage.ObjectKeyInput{
		Scope:     scope,
		Class:     storage.ObjectClassPublic,
		Namespace: mediaStorageNamespace,
		Filename:  params.Filename,
		Extension: path.Ext(params.Filename),
	})
	if err != nil {
		return PresignUploadResult{}, err
	}

	presigned, err := presigner.PresignPut(ctx, key, mimeLower, params.SizeBytes)
	if err != nil {
		return PresignUploadResult{}, err
	}
	return PresignUploadResult{
		ObjectKey: key,
		UploadURL: presigned.URL,
		Method:    presigned.Method,
		Headers:   presigned.Headers,
		ExpiresAt: presigned.ExpiresAt,
	}, nil
}

// ConfirmUpload mencatat row media setelah client selesai mengunggah lewat
// presigned URL. Object key wajib milik organization pada scope.
func (s *mediaService) ConfirmUpload(
	ctx context.Context,
	scope coretenant.Scope,
	params ConfirmUploadParams,
) (domain.LandingMediaAsset, error) {
	if s.objectStorage == nil {
		return domain.LandingMediaAsset{}, ErrStorageRequired
	}
	if err := storage.ValidateObjectOwnership(scope, params.ObjectKey); err != nil {
		return domain.LandingMediaAsset{}, err
	}
	mimeLower := strings.ToLower(params.MimeType)
	if !allowedMediaMimeTypes[mimeLower] {
		return domain.LandingMediaAsset{}, ErrInvalidMimeType
	}
	if params.SizeBytes > MaxMediaSizeBytes {
		return domain.LandingMediaAsset{}, ErrFileTooLarge
	}

	// Pastikan object benar-benar sudah terunggah sebelum membuat row.
	if reader, ok := s.objectStorage.(storage.PublicObjectReader); ok {
		body, _, err := reader.OpenPublic(ctx, params.ObjectKey)
		if err != nil {
			if errors.Is(err, storage.ErrObjectNotFound) {
				return domain.LandingMediaAsset{}, ErrUploadNotFound
			}
			return domain.LandingMediaAsset{}, err
		}
		_ = body.Close()
	}

	asset, err := s.mediaRepo.Create(ctx, scope, repository.CreateMediaAssetParams{
		StorageKey:       params.ObjectKey,
		Filename:         params.Filename,
		MimeType:         mimeLower,
		SizeBytes:        params.SizeBytes,
		AltText:          params.AltText,
		ProcessingStatus: domain.MediaProcessingCompleted,
		CreatedBy:        params.CreatedBy,
	})
	if err != nil {
		return domain.LandingMediaAsset{}, err
	}
	return s.withPublicURL(asset), nil
}

// PresignDownload mengembalikan URL bertanda tangan untuk mengunduh object milik
// organization langsung dari object storage.
func (s *mediaService) PresignDownload(
	ctx context.Context,
	scope coretenant.Scope,
	objectKey string,
) (PresignDownloadResult, error) {
	if s.objectStorage == nil {
		return PresignDownloadResult{}, ErrStorageRequired
	}
	presigner, ok := s.presigner()
	if !ok {
		return PresignDownloadResult{}, storage.ErrPresignUnsupported
	}
	if err := storage.ValidateSignedObjectRequest(storage.SignedObjectRequest{
		Scope:     scope,
		ObjectKey: objectKey,
		Operation: storage.SignedOperationDownload,
	}); err != nil {
		return PresignDownloadResult{}, err
	}

	presigned, err := presigner.PresignGet(ctx, objectKey)
	if err != nil {
		return PresignDownloadResult{}, err
	}
	return PresignDownloadResult{
		ObjectKey:   objectKey,
		DownloadURL: presigned.URL,
		Method:      presigned.Method,
		ExpiresAt:   presigned.ExpiresAt,
	}, nil
}
