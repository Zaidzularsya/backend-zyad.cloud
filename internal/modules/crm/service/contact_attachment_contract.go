package service

import (
	"context"
	"io"

	coretenant "zyad.cloud/internal/core/tenant"
	assetservice "zyad.cloud/internal/modules/asset/service"
	"zyad.cloud/internal/modules/crm/domain"
)

type UploadContactAttachmentParams struct {
	Filename   string
	MimeType   string
	SizeBytes  int64
	UploadedBy string
}

type ContactAttachmentService interface {
	List(ctx context.Context, scope coretenant.Scope, contactID string) ([]domain.ContactAttachment, error)
	Upload(ctx context.Context, scope coretenant.Scope, contactID string, params UploadContactAttachmentParams, content io.Reader) (domain.ContactAttachment, error)
	DownloadURL(ctx context.Context, scope coretenant.Scope, contactID string, attachmentID string) (assetservice.DownloadResult, error)
	Delete(ctx context.Context, scope coretenant.Scope, contactID string, attachmentID string) error
}
