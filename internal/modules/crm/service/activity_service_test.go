package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

type fakeActivityRepo struct {
	repository.ActivityRepository
	created   repository.CreateActivityParams
	found     domain.Activity
	cancelled bool
}

func (f *fakeActivityRepo) Create(_ context.Context, _ coretenant.Scope, p repository.CreateActivityParams) (domain.Activity, error) {
	f.created = p
	return domain.Activity{ID: "new", Status: p.Status}, nil
}

func (f *fakeActivityRepo) FindByID(context.Context, coretenant.Scope, string) (domain.Activity, error) {
	return f.found, nil
}

func (f *fakeActivityRepo) Cancel(context.Context, coretenant.Scope, string, string) (domain.Activity, error) {
	f.cancelled = true
	return f.found, nil
}

func newActivityServiceFixture(repo *fakeActivityRepo) ActivityService {
	leads := &fakeLeadRepo{leads: map[string]domain.Lead{"l1": {ID: "l1"}}}
	return NewActivityService(repo, leads, nil, nil, nil)
}

func TestActivityCreateRules(t *testing.T) {
	repo := &fakeActivityRepo{}
	svc := newActivityServiceFixture(repo)
	scope := testScope(t)

	_, err := svc.Create(context.Background(), scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityLead, RelatedEntityID: "l1", Type: domain.ActivityTypeTask, Subject: "x"})
	if !errors.Is(err, ErrDueAtRequired) {
		t.Fatalf("pending task without due: %v", err)
	}

	if _, err := svc.Create(context.Background(), scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityLead, RelatedEntityID: "l1", Type: domain.ActivityTypeNote, Subject: "n"}); err != nil {
		t.Fatal(err)
	}
	if repo.created.Status != domain.ActivityStatusCompleted {
		t.Fatalf("note must be forced completed, got %q", repo.created.Status)
	}

	if _, err := svc.Create(context.Background(), scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityLead, RelatedEntityID: "l1", Type: domain.ActivityTypeCall, Subject: "log",
		Status: domain.ActivityStatusCompleted}); err != nil {
		t.Fatalf("completed log without due must pass: %v", err)
	}
	if _, err := svc.Create(context.Background(), scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityLead, RelatedEntityID: "l1", Type: domain.ActivityTypeCall, Subject: "bad",
		Status: domain.ActivityStatusCancelled}); !errors.Is(err, ErrInvalidActivityStatus) {
		t.Fatalf("cancelled on create: %v", err)
	}
}

func TestActivityCancelRejectsPlaybookStep(t *testing.T) {
	stepID := "step"
	repo := &fakeActivityRepo{found: domain.Activity{ID: "a1", PlaybookStepID: &stepID}}
	svc := newActivityServiceFixture(repo)
	if _, err := svc.Cancel(context.Background(), testScope(t), "a1", "u"); !errors.Is(err, ErrPlaybookStepCancelNotAllowed) {
		t.Fatalf("err=%v", err)
	}
	if repo.cancelled {
		t.Fatal("repository Cancel must not be called")
	}
}
