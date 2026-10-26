package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	organizationservice "zyad.cloud/internal/modules/organization/service"
)

type fakeResellerStore struct {
	resellers []ResellerOrgRef
	active    map[string]map[string]bool // org -> key -> aktif
}

func (f fakeResellerStore) ListResellers(context.Context, time.Time) ([]ResellerOrgRef, error) {
	return f.resellers, nil
}

func (f fakeResellerStore) ActiveFeatureKeys(_ context.Context, orgID string, keys []string, _ time.Time) (map[string]bool, error) {
	out := map[string]bool{}
	for _, k := range keys {
		if f.active[orgID][k] {
			out[k] = true
		}
	}
	return out, nil
}

type fakeBundleWriter struct {
	calls  []string
	grants map[string][]organizationservice.FeatureGrant
}

func (w *fakeBundleWriter) GrantMissing(_ context.Context, ref ResellerOrgRef, grants []organizationservice.FeatureGrant, _ time.Time) error {
	w.calls = append(w.calls, ref.ID)
	if w.grants == nil {
		w.grants = map[string][]organizationservice.FeatureGrant{}
	}
	w.grants[ref.ID] = grants
	return nil
}

func TestPlanResellerBundleListsOnlyMissingFeatures(t *testing.T) {
	store := fakeResellerStore{
		resellers: []ResellerOrgRef{
			{ID: "org-new", Slug: "pembeli-baru", ContractRef: "ctr-1"},
			{ID: "org-partial", Slug: "pembeli-sebagian", ContractRef: "ctr-2"},
			{ID: "org-done", Slug: "pembeli-lengkap", ContractRef: "ctr-3"},
		},
		active: map[string]map[string]bool{
			"org-partial": {"landing.enabled": true, "landing.max_pages": true},
			"org-done": {"landing.enabled": true, "landing.max_pages": true,
				"landing.max_sections_per_page": true, "storage.max_bytes": true},
		},
	}
	plan, err := PlanResellerBundle(context.Background(), store, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 2 || plan.Unchanged != 1 {
		t.Fatalf("changes=%d unchanged=%d, want 2/1", len(plan.Changes), plan.Unchanged)
	}
	if got := len(plan.Changes[0].Missing); got != 4 {
		t.Fatalf("org baru harus kekurangan 4 fitur, got %d", got)
	}
	keys := map[string]bool{}
	for _, g := range plan.Changes[1].Missing {
		keys[g.Key] = true
	}
	if len(keys) != 2 || !keys["landing.max_sections_per_page"] || !keys["storage.max_bytes"] {
		t.Fatalf("org sebagian hanya kurang 2 fitur: %v", keys)
	}
}

func TestResellerBundleReportAndApply(t *testing.T) {
	store := fakeResellerStore{resellers: []ResellerOrgRef{{ID: "org-new", Slug: "pembeli-baru", ContractRef: "ctr-1"}}}
	plan, _ := PlanResellerBundle(context.Background(), store, time.Now())

	var dry bytes.Buffer
	WriteResellerBundleReport(&dry, plan, false)
	for _, want := range []string{"DRY-RUN", "pembeli-baru", "landing.max_pages", "209715200"} {
		if !strings.Contains(dry.String(), want) {
			t.Fatalf("laporan dry-run kurang %q:\n%s", want, dry.String())
		}
	}

	writer := &fakeBundleWriter{}
	n, err := ApplyResellerBundle(context.Background(), plan, writer, time.Now())
	if err != nil || n != 1 || len(writer.calls) != 1 || len(writer.grants["org-new"]) != 4 {
		t.Fatalf("apply n=%d err=%v calls=%v", n, err, writer.calls)
	}
}

func TestResellerBundleNothingToDoWritesNothing(t *testing.T) {
	plan, err := PlanResellerBundle(context.Background(), fakeResellerStore{}, time.Now())
	if err != nil || len(plan.Changes) != 0 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	writer := &fakeBundleWriter{}
	if n, err := ApplyResellerBundle(context.Background(), plan, writer, time.Now()); err != nil || n != 0 || len(writer.calls) != 0 {
		t.Fatalf("n=%d err=%v calls=%v", n, err, writer.calls)
	}
}
