package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

// fakeLeadRepo hanya mengimplementasikan yang dipakai test; method lain
// panic supaya pemakaian tak terduga langsung ketahuan.
type fakeLeadRepo struct {
	repository.LeadRepository
	leads       map[string]domain.Lead
	lastCreate  repository.CreateLeadParams
	lastUpdate  repository.UpdateLeadParams
	assignCalls int
}

func (f *fakeLeadRepo) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Lead, error) {
	lead, ok := f.leads[id]
	if !ok {
		return domain.Lead{}, pgx.ErrNoRows
	}
	return lead, nil
}

func (f *fakeLeadRepo) Create(_ context.Context, _ coretenant.Scope, params repository.CreateLeadParams) (domain.Lead, error) {
	f.lastCreate = params
	return domain.Lead{ID: "new-lead"}, nil
}

func (f *fakeLeadRepo) Update(_ context.Context, _ coretenant.Scope, id string, params repository.UpdateLeadParams) (domain.Lead, error) {
	f.lastUpdate = params
	return domain.Lead{ID: id}, nil
}

func (f *fakeLeadRepo) Assign(_ context.Context, _ coretenant.Scope, id string, _ string, _ string) (domain.Lead, error) {
	f.assignCalls++
	return domain.Lead{ID: id}, nil
}

type fakeAttachmentRepo struct {
	repository.LeadAttachmentRepository
	attachments map[string]domain.LeadAttachment
	createErr   error
	deleted     []string
	calls       *[]string
}

func (f *fakeAttachmentRepo) Create(_ context.Context, _ coretenant.Scope, params repository.CreateLeadAttachmentParams) (domain.LeadAttachment, error) {
	if f.createErr != nil {
		return domain.LeadAttachment{}, f.createErr
	}
	return domain.LeadAttachment{ID: "att-1", LeadID: params.LeadID, AssetObjectID: params.AssetObjectID}, nil
}

func (f *fakeAttachmentRepo) FindByLead(_ context.Context, _ coretenant.Scope, leadID string, id string) (domain.LeadAttachment, error) {
	attachment, ok := f.attachments[id]
	if !ok || attachment.LeadID != leadID {
		return domain.LeadAttachment{}, pgx.ErrNoRows
	}
	return attachment, nil
}

func (f *fakeAttachmentRepo) Delete(_ context.Context, _ coretenant.Scope, _ string, id string) error {
	*f.calls = append(*f.calls, "repo.delete")
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeStorage struct {
	uploadErr      error
	uploadedLabels []string
	deletedObjects []string
	calls          *[]string
}

func (f *fakeStorage) UploadObject(_ context.Context, _ coretenant.Scope, params assetservice.UploadObjectParams, _ io.Reader) (assetdomain.AssetObject, error) {
	if f.uploadErr != nil {
		return assetdomain.AssetObject{}, f.uploadErr
	}
	f.uploadedLabels = append(f.uploadedLabels, params.Label)
	return assetdomain.AssetObject{ID: "obj-1"}, nil
}

func (f *fakeStorage) DeleteObject(_ context.Context, _ coretenant.Scope, id string) error {
	*f.calls = append(*f.calls, "storage.delete")
	f.deletedObjects = append(f.deletedObjects, id)
	return nil
}

func (f *fakeStorage) DownloadURL(_ context.Context, _ coretenant.Scope, id string) (assetservice.DownloadResult, error) {
	return assetservice.DownloadResult{DownloadURL: "https://example.test/" + id}, nil
}

// testScope: service tidak memvalidasi scope (itu tugas repository), dan
// fake repository di sini mengabaikannya.
func testScope(t *testing.T) coretenant.Scope {
	t.Helper()
	return coretenant.Scope{}
}

func newAttachmentFixture() (*leadAttachmentService, *fakeAttachmentRepo, *fakeStorage, *[]string) {
	calls := []string{}
	leadRepo := &fakeLeadRepo{leads: map[string]domain.Lead{"lead-1": {ID: "lead-1"}}}
	repo := &fakeAttachmentRepo{
		attachments: map[string]domain.LeadAttachment{
			"att-1": {ID: "att-1", LeadID: "lead-1", AssetObjectID: "obj-1"},
		},
		calls: &calls,
	}
	storage := &fakeStorage{calls: &calls}
	svc := NewLeadAttachmentService(repo, leadRepo, storage).(*leadAttachmentService)
	return svc, repo, storage, &calls
}

func TestLeadAttachmentUploadLabelsObjectWithLead(t *testing.T) {
	svc, _, storage, _ := newAttachmentFixture()

	attachment, err := svc.Upload(context.Background(), testScope(t), "lead-1", UploadLeadAttachmentParams{
		Filename: "quote.pdf", MimeType: "application/pdf", SizeBytes: 10,
	}, strings.NewReader("x"))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if attachment.AssetObjectID != "obj-1" {
		t.Errorf("AssetObjectID = %q, want obj-1", attachment.AssetObjectID)
	}
	if len(storage.uploadedLabels) != 1 || storage.uploadedLabels[0] != "crm_lead:lead-1" {
		t.Errorf("uploaded labels = %v, want [crm_lead:lead-1]", storage.uploadedLabels)
	}
}

func TestLeadAttachmentUploadCleansUpObjectWhenLinkFails(t *testing.T) {
	svc, repo, storage, _ := newAttachmentFixture()
	repo.createErr = errors.New("insert failed")

	_, err := svc.Upload(context.Background(), testScope(t), "lead-1", UploadLeadAttachmentParams{
		Filename: "quote.pdf", MimeType: "application/pdf", SizeBytes: 10,
	}, strings.NewReader("x"))
	if err == nil {
		t.Fatal("Upload() error = nil, want error")
	}
	if len(storage.deletedObjects) != 1 || storage.deletedObjects[0] != "obj-1" {
		t.Errorf("deleted objects = %v, want [obj-1]", storage.deletedObjects)
	}
}

func TestLeadAttachmentUploadRejectsUnknownLead(t *testing.T) {
	svc, _, storage, _ := newAttachmentFixture()

	_, err := svc.Upload(context.Background(), testScope(t), "lead-missing", UploadLeadAttachmentParams{}, strings.NewReader("x"))
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "LEAD_NOT_FOUND" {
		t.Fatalf("Upload() error = %v, want LEAD_NOT_FOUND", err)
	}
	if len(storage.uploadedLabels) != 0 {
		t.Error("object uploaded for unknown lead")
	}
}

func TestLeadAttachmentDeleteRemovesObjectBeforeLink(t *testing.T) {
	svc, _, _, calls := newAttachmentFixture()

	if err := svc.Delete(context.Background(), testScope(t), "lead-1", "att-1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	want := []string{"storage.delete", "repo.delete"}
	if strings.Join(*calls, ",") != strings.Join(want, ",") {
		t.Errorf("call order = %v, want %v", *calls, want)
	}
}

func TestLeadAttachmentDeleteRejectsAttachmentOfOtherLead(t *testing.T) {
	svc, _, _, calls := newAttachmentFixture()
	svc.leadRepo.(*fakeLeadRepo).leads["lead-2"] = domain.Lead{ID: "lead-2"}

	err := svc.Delete(context.Background(), testScope(t), "lead-2", "att-1")
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "LEAD_ATTACHMENT_NOT_FOUND" {
		t.Fatalf("Delete() error = %v, want LEAD_ATTACHMENT_NOT_FOUND", err)
	}
	if len(*calls) != 0 {
		t.Errorf("unexpected calls = %v", *calls)
	}
}
