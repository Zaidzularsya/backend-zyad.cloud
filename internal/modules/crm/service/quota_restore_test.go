package service

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
)

// Restore brings a soft-deleted row back into the active count, so it must
// consume quota exactly like Create (delete+create+restore must not loop past
// the limit).

type restoreLeadRepo struct {
	fakeLeadRepo
	restored int
}

func (f *restoreLeadRepo) Restore(context.Context, coretenant.Scope, string, string) error {
	f.restored++
	return nil
}

type restoreCompanyRepo struct {
	quotaCompanyRepo
	restored int
}

func (f *restoreCompanyRepo) Restore(context.Context, coretenant.Scope, string, string) error {
	f.restored++
	return nil
}

type restoreDealRepo struct {
	quotaDealRepo
	restored int
}

func (f *restoreDealRepo) Restore(context.Context, coretenant.Scope, string, string) error {
	f.restored++
	return nil
}

type restorePipelineRepo struct {
	quotaPipelineRepo
	restored int
}

func (f *restorePipelineRepo) Restore(context.Context, coretenant.Scope, string, string) error {
	f.restored++
	return nil
}

func assertRestoreQuota(t *testing.T, name string, restore func(QuotaRoomRequirer) (error, func() int), key string) {
	t.Helper()
	room := &fakeRoom{limit: 0}
	err, restored := restore(room)
	if !isQuotaExceeded(err) {
		t.Fatalf("%s full: Restore() error = %v, want QUOTA_EXCEEDED", name, err)
	}
	if restored() != 0 {
		t.Fatalf("%s full: repository Restore called despite full quota", name)
	}
	if len(room.keys) != 1 || room.keys[0] != key {
		t.Fatalf("%s full: enforcer keys = %v, want [%s]", name, room.keys, key)
	}
	err, restored = restore(&fakeRoom{limit: 1})
	if err != nil || restored() != 1 {
		t.Fatalf("%s with room: err=%v restored=%d", name, err, restored())
	}
}

func TestLeadRestoreRequiresRoom(t *testing.T) {
	assertRestoreQuota(t, "lead", func(room QuotaRoomRequirer) (error, func() int) {
		repo := &restoreLeadRepo{}
		svc := NewLeadService(repo, nil, nil, WithLeadQuotaEnforcer(room))
		return svc.Restore(context.Background(), testScope(t), "l1", "u1"), func() int { return repo.restored }
	}, domain.FeatureCRMMaxLeads)
}

func TestCompanyRestoreRequiresRoom(t *testing.T) {
	assertRestoreQuota(t, "company", func(room QuotaRoomRequirer) (error, func() int) {
		repo := &restoreCompanyRepo{}
		svc := NewCompanyService(repo, WithCompanyQuotaEnforcer(room))
		return svc.Restore(context.Background(), testScope(t), "c1", "u1"), func() int { return repo.restored }
	}, domain.FeatureCRMMaxCompanies)
}

func TestDealRestoreRequiresRoom(t *testing.T) {
	assertRestoreQuota(t, "deal", func(room QuotaRoomRequirer) (error, func() int) {
		repo := &restoreDealRepo{}
		svc := NewDealService(repo, &fakePipelineRepo{}, WithDealQuotaEnforcer(room))
		return svc.Restore(context.Background(), testScope(t), "d1", "u1"), func() int { return repo.restored }
	}, domain.FeatureCRMMaxDeals)
}

func TestPipelineRestoreRequiresRoom(t *testing.T) {
	assertRestoreQuota(t, "pipeline", func(room QuotaRoomRequirer) (error, func() int) {
		repo := &restorePipelineRepo{quotaPipelineRepo: quotaPipelineRepo{active: 1}}
		svc := NewPipelineService(repo, WithPipelineQuotaEnforcer(room))
		return svc.Restore(context.Background(), testScope(t), "p2", "u1"), func() int { return repo.restored }
	}, domain.FeatureCRMMaxPipelines)
}

// Same "first pipeline is free" rule as Create: with no active pipeline the
// restored one is the organization's only pipeline.
func TestPipelineRestoreOfOnlyPipelineIsFree(t *testing.T) {
	repo := &restorePipelineRepo{quotaPipelineRepo: quotaPipelineRepo{active: 0}}
	room := &fakeRoom{limit: 0}
	svc := NewPipelineService(repo, WithPipelineQuotaEnforcer(room))
	if err := svc.Restore(context.Background(), testScope(t), "p1", "u1"); err != nil || repo.restored != 1 {
		t.Fatalf("err=%v restored=%d", err, repo.restored)
	}
	if len(room.keys) != 0 {
		t.Fatalf("enforcer keys = %v", room.keys)
	}
}

// A lead leaving a terminal status (unqualified/converted) through Update
// re-enters the active lead count, so it needs room; edits that keep the lead
// in the working set do not.
func TestLeadUpdateTerminalToActiveRequiresRoom(t *testing.T) {
	for _, from := range []domain.LeadStatus{domain.LeadStatusUnqualified, domain.LeadStatusConverted} {
		repo := &fakeLeadRepo{leads: map[string]domain.Lead{"l1": {ID: "l1", Status: from}}}
		room := &fakeRoom{limit: 0}
		svc := NewLeadService(repo, nil, nil, WithLeadQuotaEnforcer(room))
		status := domain.LeadStatusNew
		_, err := svc.Update(context.Background(), testScope(t), "l1", repository.UpdateLeadParams{Status: &status})
		if !isQuotaExceeded(err) {
			t.Fatalf("%s->new: error = %v, want QUOTA_EXCEEDED", from, err)
		}
		if repo.lastUpdate.Status != nil {
			t.Fatalf("%s->new: repository Update called despite full quota", from)
		}
		if _, err := NewLeadService(repo, nil, nil, WithLeadQuotaEnforcer(&fakeRoom{limit: 1})).
			Update(context.Background(), testScope(t), "l1", repository.UpdateLeadParams{Status: &status}); err != nil {
			t.Fatalf("%s->new with room: %v", from, err)
		}
	}
}

func TestLeadUpdateActiveStatusChangeIgnoresQuota(t *testing.T) {
	repo := &fakeLeadRepo{leads: map[string]domain.Lead{"l1": {ID: "l1", Status: domain.LeadStatusNew}}}
	room := &fakeRoom{limit: 0}
	svc := NewLeadService(repo, nil, nil, WithLeadQuotaEnforcer(room))
	status := domain.LeadStatusContacted
	if _, err := svc.Update(context.Background(), testScope(t), "l1", repository.UpdateLeadParams{Status: &status}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(room.keys) != 0 {
		t.Fatalf("enforcer consulted for a non-terminal transition: %v", room.keys)
	}
}
