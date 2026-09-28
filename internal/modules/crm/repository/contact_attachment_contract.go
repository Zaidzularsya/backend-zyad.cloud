package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type CreateContactAttachmentParams struct {
	ContactID     string
	AssetObjectID string
	CreatedBy     string
}

// ContactAttachmentRepository hanya mengelola baris link. Upload/hapus file
// fisik dan kuota storage tetap lewat modul asset.
type ContactAttachmentRepository interface {
	Create(context.Context, coretenant.Scope, CreateContactAttachmentParams) (domain.ContactAttachment, error)
	// ListByContact hanya mengembalikan attachment yang file-nya belum dihapus
	// dari storage tenant (asset_objects.deleted_at IS NULL).
	ListByContact(ctx context.Context, scope coretenant.Scope, contactID string) ([]domain.ContactAttachment, error)
	FindByContact(ctx context.Context, scope coretenant.Scope, contactID string, id string) (domain.ContactAttachment, error)
	Delete(ctx context.Context, scope coretenant.Scope, contactID string, id string) error
}
