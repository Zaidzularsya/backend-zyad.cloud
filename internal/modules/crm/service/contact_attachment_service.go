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

type contactAttachmentService struct {
	repo        repository.ContactAttachmentRepository
	contactRepo repository.ContactRepository
	storage     AttachmentStorage
}

func NewContactAttachmentService(
	repo repository.ContactAttachmentRepository,
	contactRepo repository.ContactRepository,
	storage AttachmentStorage,
) ContactAttachmentService {
	return &contactAttachmentService{repo: repo, contactRepo: contactRepo, storage: storage}
}

func (s *contactAttachmentService) requireContact(ctx context.Context, scope coretenant.Scope, contactID string) error {
	_, err := s.contactRepo.FindByID(ctx, scope, contactID)
	return crmmodule.MapNotFound(err, "CONTACT_NOT_FOUND", "contact not found or already deleted")
}

func (s *contactAttachmentService) List(ctx context.Context, scope coretenant.Scope, contactID string) ([]domain.ContactAttachment, error) {
	if err := s.requireContact(ctx, scope, contactID); err != nil {
		return nil, err
	}
	return s.repo.ListByContact(ctx, scope, contactID)
}

func (s *contactAttachmentService) Upload(
	ctx context.Context,
	scope coretenant.Scope,
	contactID string,
	params UploadContactAttachmentParams,
	content io.Reader,
) (domain.ContactAttachment, error) {
	if err := s.requireContact(ctx, scope, contactID); err != nil {
		return domain.ContactAttachment{}, err
	}

	object, err := s.storage.UploadObject(ctx, scope, assetservice.UploadObjectParams{
		Filename:  params.Filename,
		MimeType:  params.MimeType,
		SizeBytes: params.SizeBytes,
		Class:     assetdomain.ObjectClassPrivate,
		Label:     "crm_contact:" + contactID,
		CreatedBy: params.UploadedBy,
	}, content)
	if err != nil {
		return domain.ContactAttachment{}, err
	}

	attachment, err := s.repo.Create(ctx, scope, repository.CreateContactAttachmentParams{
		ContactID:     contactID,
		AssetObjectID: object.ID,
		CreatedBy:     params.UploadedBy,
	})
	if err != nil {
		// Link gagal dibuat — hapus object-nya supaya tidak memakan kuota
		// storage tanpa bisa diakses dari contact.
		_ = s.storage.DeleteObject(ctx, scope, object.ID)
		return domain.ContactAttachment{}, err
	}
	return attachment, nil
}

func (s *contactAttachmentService) DownloadURL(ctx context.Context, scope coretenant.Scope, contactID string, attachmentID string) (assetservice.DownloadResult, error) {
	attachment, err := s.findAttachment(ctx, scope, contactID, attachmentID)
	if err != nil {
		return assetservice.DownloadResult{}, err
	}
	return s.storage.DownloadURL(ctx, scope, attachment.AssetObjectID)
}

// Delete menghapus object storage lebih dulu, baru link-nya. Kalau langkah
// kedua gagal, link yatim tetap tersembunyi karena ListByContact menyaring
// object yang sudah dihapus — kebalikannya (link hilang, file tertinggal)
// akan diam-diam memakan kuota storage tenant.
func (s *contactAttachmentService) Delete(ctx context.Context, scope coretenant.Scope, contactID string, attachmentID string) error {
	attachment, err := s.findAttachment(ctx, scope, contactID, attachmentID)
	if err != nil {
		return err
	}
	if err := s.storage.DeleteObject(ctx, scope, attachment.AssetObjectID); err != nil {
		return err
	}
	return crmmodule.MapNotFound(
		s.repo.Delete(ctx, scope, contactID, attachmentID),
		"CONTACT_ATTACHMENT_NOT_FOUND", "attachment not found",
	)
}

func (s *contactAttachmentService) findAttachment(ctx context.Context, scope coretenant.Scope, contactID string, attachmentID string) (domain.ContactAttachment, error) {
	if err := s.requireContact(ctx, scope, contactID); err != nil {
		return domain.ContactAttachment{}, err
	}
	attachment, err := s.repo.FindByContact(ctx, scope, contactID, attachmentID)
	if err != nil {
		return domain.ContactAttachment{}, crmmodule.MapNotFound(err, "CONTACT_ATTACHMENT_NOT_FOUND", "attachment not found")
	}
	return attachment, nil
}
