package service

import (
	"context"
	"io"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type MediaService interface {
	UploadAsset(ctx context.Context, scope coretenant.Scope, params repository.CreateMediaAssetParams, content io.Reader) (domain.LandingMediaAsset, error)
	GetAsset(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingMediaAsset, error)
	ListAssets(ctx context.Context, scope coretenant.Scope) ([]domain.LandingMediaAsset, error)
	DeleteAsset(ctx context.Context, scope coretenant.Scope, id string) error
	MarkAssetProcessing(ctx context.Context, scope coretenant.Scope, id string, status domain.MediaProcessingStatus) error
}
