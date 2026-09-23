package service

import (
	"context"
	"io"

	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	"zyad.cloud/internal/modules/crm/domain"
)

// AttachmentStorage adalah subset assetservice.AssetService. File lampiran
// lead disimpan sebagai object storage tenant biasa, jadi batas MIME/ukuran,
// kuota storage.max_bytes, dan presign download mengikuti modul asset.
type AttachmentStorage interface {
	UploadObject(ctx context.Context, scope coretenant.Scope, params assetservice.UploadObjectParams, content io.Reader) (assetdomain.AssetObject, error)
	DeleteObject(ctx context.Context, scope coretenant.Scope, id string) error
	DownloadURL(ctx context.Context, scope coretenant.Scope, id string) (assetservice.DownloadResult, error)
}

type UploadLeadAttachmentParams struct {
	Filename   string
	MimeType   string
	SizeBytes  int64
	UploadedBy string
}

type LeadAttachmentService interface {
	List(ctx context.Context, scope coretenant.Scope, leadID string) ([]domain.LeadAttachment, error)
	Upload(ctx context.Context, scope coretenant.Scope, leadID string, params UploadLeadAttachmentParams, content io.Reader) (domain.LeadAttachment, error)
	DownloadURL(ctx context.Context, scope coretenant.Scope, leadID string, attachmentID string) (assetservice.DownloadResult, error)
	Delete(ctx context.Context, scope coretenant.Scope, leadID string, attachmentID string) error
}
