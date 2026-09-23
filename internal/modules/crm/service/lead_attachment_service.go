package service

import (
	"context"
	"io"

	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	crmmodule "zyad.cloud/internal/modules/crm"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type leadAttachmentService struct {
	repo     repository.LeadAttachmentRepository
	leadRepo repository.LeadRepository
	storage  AttachmentStorage
}

func NewLeadAttachmentService(
	repo repository.LeadAttachmentRepository,
	leadRepo repository.LeadRepository,
	storage AttachmentStorage,
) LeadAttachmentService {
	return &leadAttachmentService{repo: repo, leadRepo: leadRepo, storage: storage}
}

func (s *leadAttachmentService) requireLead(ctx context.Context, scope coretenant.Scope, leadID string) error {
	_, err := s.leadRepo.FindByID(ctx, scope, leadID)
	return crmmodule.MapNotFound(err, "LEAD_NOT_FOUND", "lead not found or already deleted")
}

func (s *leadAttachmentService) List(ctx context.Context, scope coretenant.Scope, leadID string) ([]domain.LeadAttachment, error) {
	if err := s.requireLead(ctx, scope, leadID); err != nil {
		return nil, err
	}
	return s.repo.ListByLead(ctx, scope, leadID)
}

func (s *leadAttachmentService) Upload(
	ctx context.Context,
	scope coretenant.Scope,
	leadID string,
	params UploadLeadAttachmentParams,
	content io.Reader,
) (domain.LeadAttachment, error) {
	if err := s.requireLead(ctx, scope, leadID); err != nil {
		return domain.LeadAttachment{}, err
	}

	object, err := s.storage.UploadObject(ctx, scope, assetservice.UploadObjectParams{
		Filename:  params.Filename,
		MimeType:  params.MimeType,
		SizeBytes: params.SizeBytes,
		Class:     assetdomain.ObjectClassPrivate,
		Label:     "crm_lead:" + leadID,
		CreatedBy: params.UploadedBy,
	}, content)
	if err != nil {
		return domain.LeadAttachment{}, err
	}

	attachment, err := s.repo.Create(ctx, scope, repository.CreateLeadAttachmentParams{
		LeadID:        leadID,
		AssetObjectID: object.ID,
		CreatedBy:     params.UploadedBy,
	})
	if err != nil {
		// Link gagal dibuat — hapus object-nya supaya tidak memakan kuota
		// storage tanpa bisa diakses dari lead.
		_ = s.storage.DeleteObject(ctx, scope, object.ID)
		return domain.LeadAttachment{}, err
	}
	return attachment, nil
}

func (s *leadAttachmentService) DownloadURL(ctx context.Context, scope coretenant.Scope, leadID string, attachmentID string) (assetservice.DownloadResult, error) {
	attachment, err := s.findAttachment(ctx, scope, leadID, attachmentID)
	if err != nil {
		return assetservice.DownloadResult{}, err
	}
	return s.storage.DownloadURL(ctx, scope, attachment.AssetObjectID)
}

// Delete menghapus object storage lebih dulu, baru link-nya. Kalau langkah
// kedua gagal, link yatim tetap tersembunyi karena ListByLead menyaring
// object yang sudah dihapus — kebalikannya (link hilang, file tertinggal)
// akan diam-diam memakan kuota storage tenant.
func (s *leadAttachmentService) Delete(ctx context.Context, scope coretenant.Scope, leadID string, attachmentID string) error {
	attachment, err := s.findAttachment(ctx, scope, leadID, attachmentID)
	if err != nil {
		return err
	}
	if err := s.storage.DeleteObject(ctx, scope, attachment.AssetObjectID); err != nil {
		return err
	}
	return crmmodule.MapNotFound(
		s.repo.Delete(ctx, scope, leadID, attachmentID),
		"LEAD_ATTACHMENT_NOT_FOUND", "attachment not found",
	)
}

func (s *leadAttachmentService) findAttachment(ctx context.Context, scope coretenant.Scope, leadID string, attachmentID string) (domain.LeadAttachment, error) {
	if err := s.requireLead(ctx, scope, leadID); err != nil {
		return domain.LeadAttachment{}, err
	}
	attachment, err := s.repo.FindByLead(ctx, scope, leadID, attachmentID)
	if err != nil {
		return domain.LeadAttachment{}, crmmodule.MapNotFound(err, "LEAD_ATTACHMENT_NOT_FOUND", "attachment not found")
	}
	return attachment, nil
}
