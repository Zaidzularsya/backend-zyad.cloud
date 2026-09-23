package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
)

type CreateLeadAttachmentParams struct {
	LeadID        string
	AssetObjectID string
	CreatedBy     string
}

// LeadAttachmentRepository hanya mengelola baris link. Upload/hapus file
// fisik dan kuota storage tetap lewat modul asset.
type LeadAttachmentRepository interface {
	Create(context.Context, coretenant.Scope, CreateLeadAttachmentParams) (domain.LeadAttachment, error)
	// ListByLead hanya mengembalikan attachment yang file-nya belum dihapus
	// dari storage tenant (asset_objects.deleted_at IS NULL).
	ListByLead(ctx context.Context, scope coretenant.Scope, leadID string) ([]domain.LeadAttachment, error)
	FindByLead(ctx context.Context, scope coretenant.Scope, leadID string, id string) (domain.LeadAttachment, error)
	Delete(ctx context.Context, scope coretenant.Scope, leadID string, id string) error
}
