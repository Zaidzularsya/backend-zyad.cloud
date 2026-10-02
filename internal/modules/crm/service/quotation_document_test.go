package service

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/quotationpdf"
	"zyad.cloud/internal/modules/crm/repository"
)

type fakeIssuer struct{}

func (fakeIssuer) Find(context.Context, coretenant.Scope) (quotationpdf.Issuer, error) {
	return quotationpdf.Issuer{Name: "PT Zyad"}, nil
}

type fakeRenderer struct{ docs []quotationpdf.Document }

func (f *fakeRenderer) Render(d quotationpdf.Document) ([]byte, error) {
	f.docs = append(f.docs, d)
	if d.Draft {
		return []byte("%PDF-draft"), nil
	}
	return []byte("%PDF-final"), nil
}

type fakeFiles struct {
	uploads int
	data    map[string][]byte
}

func (f *fakeFiles) UploadObject(_ context.Context, _ coretenant.Scope, _ assetservice.UploadObjectParams, r io.Reader) (assetdomain.AssetObject, error) {
	f.uploads++
	b, _ := io.ReadAll(r)
	f.data["a1"] = b
	return assetdomain.AssetObject{ID: "a1"}, nil
}
func (f *fakeFiles) OpenObject(_ context.Context, _ coretenant.Scope, id string) (io.ReadCloser, assetdomain.AssetObject, error) {
	b, ok := f.data[id]
	if !ok {
		return nil, assetdomain.AssetObject{}, pgx.ErrNoRows
	}
	return io.NopCloser(bytes.NewReader(b)), assetdomain.AssetObject{ID: id}, nil
}
func (f *fakeFiles) DeleteObject(context.Context, coretenant.Scope, string) error { return nil }

type snapshotRepo struct {
	fakeQuotationRepo
	snapshotSet bool
	sent        bool
}

func (r *snapshotRepo) SetPDFSnapshot(_ context.Context, _ coretenant.Scope, id, asset string) (bool, error) {
	q := r.stored[id]
	if q.PDFAssetID != nil {
		return false, nil
	}
	q.PDFAssetID = &asset
	r.stored[id] = q
	r.snapshotSet = true
	return true, nil
}
func (r *snapshotRepo) Send(_ context.Context, _ coretenant.Scope, id, _ string) (domain.Quotation, error) {
	q := r.stored[id]
	q.Status = domain.QuotationStatusSent
	r.stored[id] = q
	r.sent = true
	return q, nil
}

type fakeActivities struct {
	repository.ActivityRepository
	created []repository.CreateActivityParams
}

func (f *fakeActivities) Create(_ context.Context, _ coretenant.Scope, p repository.CreateActivityParams) (domain.Activity, error) {
	f.created = append(f.created, p)
	return domain.Activity{}, nil
}

func docFixture() (*quotationService, *snapshotRepo, *fakeFiles, *fakeRenderer, *fakeActivities) {
	deal := "d1"
	repo := &snapshotRepo{fakeQuotationRepo: fakeQuotationRepo{stored: map[string]domain.Quotation{
		"q1": {ID: "q1", QuotationNumber: "QUO-2026-0001", Status: domain.QuotationStatusDraft, DealID: &deal, Subtotal: "1.00", DiscountTotal: "0.00", TaxTotal: "0.00", GrandTotal: "1.00"},
	}}}
	files := &fakeFiles{data: map[string][]byte{}}
	renderer := &fakeRenderer{}
	acts := &fakeActivities{}
	svc := NewQuotationService(repo, fakeCounter{}, WithQuotationDocuments(QuotationDocumentDeps{
		Issuers: fakeIssuer{}, Files: files, Renderer: renderer, Activities: acts,
	})).(*quotationService)
	return svc, repo, files, renderer, acts
}

func TestDraftPDFIsRenderedNotStored(t *testing.T) {
	svc, repo, files, renderer, _ := docFixture()
	pdf, err := svc.PDF(context.Background(), coretenant.Scope{}, "q1")
	if err != nil || string(pdf.Content) != "%PDF-draft" || pdf.Filename != "QUO-2026-0001.pdf" {
		t.Fatalf("pdf = %+v err=%v", pdf, err)
	}
	if files.uploads != 0 || repo.snapshotSet || !renderer.docs[0].Draft || renderer.docs[0].Issuer.Name != "PT Zyad" {
		t.Fatal("draft must not be stored")
	}
}

func TestMarkSentStoresSnapshotOnceAndLogsDealActivity(t *testing.T) {
	svc, repo, files, _, acts := docFixture()
	q, err := svc.MarkSentManually(context.Background(), coretenant.Scope{}, "q1", "u1")
	if err != nil || q.Status != domain.QuotationStatusSent || !repo.sent {
		t.Fatalf("q = %+v err=%v", q, err)
	}
	if files.uploads != 1 || string(files.data["a1"]) != "%PDF-final" {
		t.Fatalf("uploads = %d", files.uploads)
	}
	if len(acts.created) != 1 || acts.created[0].RelatedEntityID != "d1" || acts.created[0].Status != domain.ActivityStatusCompleted {
		t.Fatalf("activities = %+v", acts.created)
	}
	// PDF berikutnya dilayani dari snapshot, tanpa upload/render ulang.
	pdf, err := svc.PDF(context.Background(), coretenant.Scope{}, "q1")
	if err != nil || string(pdf.Content) != "%PDF-final" || files.uploads != 1 {
		t.Fatalf("pdf = %q uploads = %d err=%v", pdf.Content, files.uploads, err)
	}
}
