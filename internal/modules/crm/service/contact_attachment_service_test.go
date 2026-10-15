package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

// fakeContactRepo hanya mengimplementasikan FindByID; method lain panic.
type fakeContactRepo struct {
	repository.ContactRepository
	contacts map[string]domain.Contact
}

func (f *fakeContactRepo) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Contact, error) {
	contact, ok := f.contacts[id]
	if !ok {
		return domain.Contact{}, pgx.ErrNoRows
	}
	return contact, nil
}

type fakeContactAttachmentRepo struct {
	repository.ContactAttachmentRepository
	attachments map[string]domain.ContactAttachment
	createErr   error
	calls       *[]string
}

func (f *fakeContactAttachmentRepo) Create(_ context.Context, _ coretenant.Scope, params repository.CreateContactAttachmentParams) (domain.ContactAttachment, error) {
	if f.createErr != nil {
		return domain.ContactAttachment{}, f.createErr
	}
	return domain.ContactAttachment{ID: "att-1", ContactID: params.ContactID, AssetObjectID: params.AssetObjectID}, nil
}

func (f *fakeContactAttachmentRepo) FindByContact(_ context.Context, _ coretenant.Scope, contactID string, id string) (domain.ContactAttachment, error) {
	attachment, ok := f.attachments[id]
	if !ok || attachment.ContactID != contactID {
		return domain.ContactAttachment{}, pgx.ErrNoRows
	}
	return attachment, nil
}

func (f *fakeContactAttachmentRepo) Delete(_ context.Context, _ coretenant.Scope, _ string, _ string) error {
	*f.calls = append(*f.calls, "repo.delete")
	return nil
}

func newContactAttachmentFixture() (*contactAttachmentService, *fakeContactAttachmentRepo, *fakeStorage, *[]string) {
	calls := []string{}
	contactRepo := &fakeContactRepo{contacts: map[string]domain.Contact{"contact-1": {ID: "contact-1"}}}
	repo := &fakeContactAttachmentRepo{
		attachments: map[string]domain.ContactAttachment{
			"att-1": {ID: "att-1", ContactID: "contact-1", AssetObjectID: "obj-1"},
		},
		calls: &calls,
	}
	storage := &fakeStorage{calls: &calls}
	svc := NewContactAttachmentService(repo, contactRepo, storage).(*contactAttachmentService)
	return svc, repo, storage, &calls
}

func TestContactAttachmentUploadLabelsObjectWithContact(t *testing.T) {
	svc, _, storage, _ := newContactAttachmentFixture()

	_, err := svc.Upload(context.Background(), testScope(t), "contact-1", UploadContactAttachmentParams{
		Filename: "ktp.pdf", MimeType: "application/pdf", SizeBytes: 10,
	}, strings.NewReader("x"))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if len(storage.uploadedLabels) != 1 || storage.uploadedLabels[0] != "crm_contact:contact-1" {
		t.Errorf("uploaded labels = %v, want [crm_contact:contact-1]", storage.uploadedLabels)
	}
}

func TestContactAttachmentUploadCleansUpObjectWhenLinkFails(t *testing.T) {
	svc, repo, storage, _ := newContactAttachmentFixture()
	repo.createErr = errors.New("insert failed")

	_, err := svc.Upload(context.Background(), testScope(t), "contact-1", UploadContactAttachmentParams{}, strings.NewReader("x"))
	if err == nil {
		t.Fatal("Upload() error = nil, want error")
	}
	if len(storage.deletedObjects) != 1 || storage.deletedObjects[0] != "obj-1" {
		t.Errorf("deleted objects = %v, want [obj-1]", storage.deletedObjects)
	}
}

func TestContactAttachmentUploadRejectsUnknownContact(t *testing.T) {
	svc, _, storage, _ := newContactAttachmentFixture()

	_, err := svc.Upload(context.Background(), testScope(t), "contact-missing", UploadContactAttachmentParams{}, strings.NewReader("x"))
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "CONTACT_NOT_FOUND" {
		t.Fatalf("Upload() error = %v, want CONTACT_NOT_FOUND", err)
	}
	if len(storage.uploadedLabels) != 0 {
		t.Error("object uploaded for unknown contact")
	}
}

func TestContactAttachmentDeleteRejectsAttachmentOfOtherContact(t *testing.T) {
	svc, _, _, calls := newContactAttachmentFixture()
	svc.contactRepo.(*fakeContactRepo).contacts["contact-2"] = domain.Contact{ID: "contact-2"}

	err := svc.Delete(context.Background(), testScope(t), "contact-2", "att-1")
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "CONTACT_ATTACHMENT_NOT_FOUND" {
		t.Fatalf("Delete() error = %v, want CONTACT_ATTACHMENT_NOT_FOUND", err)
	}
	if len(*calls) != 0 {
		t.Errorf("unexpected calls = %v", *calls)
	}
}
