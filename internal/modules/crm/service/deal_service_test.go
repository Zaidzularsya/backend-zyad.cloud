package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

// recordingDealRepo stores the last create/update params.
type recordingDealRepo struct {
	repository.DealRepository
	created domain.Deal
	update  repository.UpdateDealParams
}

func (r *recordingDealRepo) Create(_ context.Context, _ coretenant.Scope, p repository.CreateDealParams) (domain.Deal, error) {
	contactID := p.ContactID
	r.created = domain.Deal{ID: "d-new", ContactID: &contactID, Title: p.Title, Source: p.Source}
	return r.created, nil
}

func (r *recordingDealRepo) FindByID(context.Context, coretenant.Scope, string) (domain.Deal, error) {
	return r.created, nil
}

func (r *recordingDealRepo) Update(_ context.Context, _ coretenant.Scope, id string, p repository.UpdateDealParams) (domain.Deal, error) {
	r.update = p
	return domain.Deal{ID: id, ContactID: p.ContactID}, nil
}

func dealSvcFixture() (DealService, *recordingDealRepo) {
	deals := &recordingDealRepo{}
	pipelines := &fakePipelineRepo{pipelines: map[string]domain.Pipeline{
		"p1": {ID: "p1", Stages: []domain.PipelineStage{{ID: "s1"}}},
	}}
	contacts := &fakeContactRepo{contacts: map[string]domain.Contact{"c1": {ID: "c1"}}}
	return NewDealService(deals, pipelines, WithDealContactSync(contacts)), deals
}

func TestDealUpdate_SetsContact(t *testing.T) {
	svc, deals := dealSvcFixture()

	got, err := svc.Update(context.Background(), testScope(t), "d1", repository.UpdateDealParams{ContactID: strPtr("c1")})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.ContactID == nil || *got.ContactID != "c1" || deals.update.ContactID == nil {
		t.Fatalf("contact not stored: %+v", got)
	}

	// A contact of another organization is invisible to the scoped lookup.
	_, err = svc.Update(context.Background(), testScope(t), "d1", repository.UpdateDealParams{ContactID: strPtr("c-other-org")})
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "CONTACT_NOT_FOUND" || appErr.Status != http.StatusNotFound {
		t.Fatalf("Update(other org contact) error = %v, want CONTACT_NOT_FOUND", err)
	}

	// Clearing the contact (empty string) needs no lookup.
	if _, err := svc.Update(context.Background(), testScope(t), "d1", repository.UpdateDealParams{ContactID: strPtr("")}); err != nil {
		t.Fatalf("clearing contact error = %v", err)
	}
}

func TestDealCreate_StoresSource(t *testing.T) {
	svc, _ := dealSvcFixture()

	got, err := svc.Create(context.Background(), testScope(t), repository.CreateDealParams{
		PipelineID: "p1", StageID: "s1", ContactID: "c1", Title: "Internet 10M", Source: "Impor PPPoE",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.Source != "Impor PPPoE" {
		t.Fatalf("Source = %q", got.Source)
	}

	_, err = svc.Create(context.Background(), testScope(t), repository.CreateDealParams{
		PipelineID: "p1", StageID: "s1", ContactID: "missing", Title: "X",
	})
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "CONTACT_NOT_FOUND" {
		t.Fatalf("Create(missing contact) error = %v, want CONTACT_NOT_FOUND", err)
	}
}
