package service

import (
	"context"
	"errors"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
)

type fakeOrgs struct {
	ids   []string
	calls [][2]int
}

func (f *fakeOrgs) ListActive(_ context.Context, limit, offset int) ([]string, error) {
	f.calls = append(f.calls, [2]int{limit, offset})
	if offset >= len(f.ids) {
		return nil, nil
	}
	return f.ids[offset:min(offset+limit, len(f.ids))], nil
}

type runnerScopes struct{ failFor map[string]bool }

func (s runnerScopes) ResolveWorkerOrganization(_ context.Context, orgID, identity string) (coretenant.Context, error) {
	if identity != OverdueWorkerIdentity {
		return coretenant.Context{}, errors.New("wrong identity " + identity)
	}
	return s.resolve(orgID)
}

func (s runnerScopes) resolve(orgID string) (coretenant.Context, error) {
	if s.failFor[orgID] {
		return coretenant.Context{}, errors.New("organization inactive")
	}
	return coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: orgID, OrganizationSlug: "o", OrganizationType: coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive, ResolutionSource: coretenant.ResolutionSourceWorker, DataPlacement: coretenant.DataPlacementShared,
	})
}

func TestOverdueRunnerVisitsEveryOrganizationAcrossPages(t *testing.T) {
	store := newFakeStore()
	orgs := &fakeOrgs{ids: []string{
		"11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333",
	}}
	r := NewOverdueRunner(orgs, runnerScopes{}, store, func() time.Time { return nowWIB })
	r.pageSize = 2
	res, err := r.RunOnce(ctx)
	if err != nil || res.Checked != 3 || res.Marked != 3 || res.Failed != 0 {
		t.Fatalf("result = %+v err=%v", res, err)
	}
	if store.overdueSeen.Format("2006-01-02") != "2026-10-05" {
		t.Fatalf("today = %v, want the WIB business day", store.overdueSeen)
	}
	if len(orgs.calls) != 2 || orgs.calls[1] != [2]int{2, 2} {
		t.Fatalf("paging calls = %v", orgs.calls)
	}
}

func TestOverdueRunnerBusinessDayAroundMidnight(t *testing.T) {
	store := newFakeStore()
	late := time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC) // 03:00 WIB 6 Okt
	r := NewOverdueRunner(&fakeOrgs{ids: []string{"11111111-1111-1111-1111-111111111111"}}, runnerScopes{}, store, func() time.Time { return late })
	if _, err := r.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if store.overdueSeen.Format("2006-01-02") != "2026-10-06" {
		t.Fatalf("today = %v", store.overdueSeen)
	}
}

func TestOverdueRunnerKeepsGoingAfterOneOrganizationFails(t *testing.T) {
	store := newFakeStore()
	bad := "22222222-2222-2222-2222-222222222222"
	orgs := &fakeOrgs{ids: []string{"11111111-1111-1111-1111-111111111111", bad, "33333333-3333-3333-3333-333333333333"}}
	r := NewOverdueRunner(orgs, runnerScopes{failFor: map[string]bool{bad: true}}, store, func() time.Time { return nowWIB })
	res, err := r.RunOnce(ctx)
	if err != nil || res.Checked != 3 || res.Failed != 1 || res.Marked != 2 {
		t.Fatalf("result = %+v err=%v", res, err)
	}
}

func TestOverdueRunnerStopsWhenContextIsCancelled(t *testing.T) {
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	r := NewOverdueRunner(&fakeOrgs{ids: []string{"11111111-1111-1111-1111-111111111111"}}, runnerScopes{}, newFakeStore(), func() time.Time { return nowWIB })
	if _, err := r.RunOnce(cctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
