package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type CreateMediaAssetParams struct {
	StorageKey       string
	Filename         string
	MimeType         string
	SizeBytes        int64
	Width            *int
	Height           *int
	DurationSeconds  *int
	AltText          string
	ProcessingStatus domain.MediaProcessingStatus
	CreatedBy        string
}

type MediaRepository interface {
	Create(context.Context, coretenant.Scope, CreateMediaAssetParams) (domain.LandingMediaAsset, error)
	Get(context.Context, coretenant.Scope, string) (domain.LandingMediaAsset, error)
	List(context.Context, coretenant.Scope) ([]domain.LandingMediaAsset, error)
	UpdateStatus(context.Context, coretenant.Scope, string, domain.MediaProcessingStatus) error
	Delete(context.Context, coretenant.Scope, string) error
}
