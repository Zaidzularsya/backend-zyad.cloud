package service

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"

	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

// fakeContractEntStore meniru perilaku repository yang relevan: Upsert per
// (org, fitur, source, referensi) dan version naik hanya bila nilai berubah.
type fakeContractEntStore struct {
	rows    []*model.Entitlement
	upserts int
	nextID  int
}

func (f *fakeContractEntStore) Upsert(_ context.Context, p repository.UpsertEntitlementParams) (model.Entitlement, error) {
	f.upserts++
	var stored map[string]any // seperti kolom jsonb: angka kembali sebagai float64
	raw, _ := json.Marshal(p.Limits)
	_ = json.Unmarshal(raw, &stored)
	p.Limits = stored
	status := p.Status
	if status == "" {
		status = model.EntitlementStatusActive
	}
	for _, r := range f.rows {
		if r.OrganizationID == p.OrganizationID && r.FeatureKey == p.FeatureKey && r.Source == p.Source && r.SourceReference == p.SourceReference {
			if r.Status != status || !reflect.DeepEqual(r.Limits, p.Limits) || !r.EffectiveFrom.Equal(p.EffectiveFrom) {
				r.Version++
			}
			r.Status, r.Limits, r.EffectiveFrom, r.EffectiveUntil, r.Reason = status, p.Limits, p.EffectiveFrom, p.EffectiveUntil, p.Reason
			return *r, nil
		}
	}
	f.nextID++
	row := &model.Entitlement{
		ID: string(rune('a' + f.nextID)), OrganizationID: p.OrganizationID, FeatureKey: p.FeatureKey, Source: p.Source,
		SourceReference: p.SourceReference, Status: status, Limits: p.Limits, Version: 1,
		EffectiveFrom: p.EffectiveFrom, EffectiveUntil: p.EffectiveUntil, Reason: p.Reason,
	}
	f.rows = append(f.rows, row)
	return *row, nil
}

func (f *fakeContractEntStore) ListActiveBySource(_ context.Context, org string, source model.EntitlementSource, ref *string, _ time.Time) ([]model.Entitlement, error) {
	var out []model.Entitlement
	for _, r := range f.rows {
		if r.OrganizationID == org && r.Source == source && r.Status == model.EntitlementStatusActive && (ref == nil || r.SourceReference == *ref) {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (f *fakeContractEntStore) HasActiveSource(ctx context.Context, org string, source model.EntitlementSource, at time.Time) (bool, error) {
	rows, _ := f.ListActiveBySource(ctx, org, source, nil, at)
	return len(rows) > 0, nil
}

func (f *fakeContractEntStore) ExpireBySource(_ context.Context, org string, source model.EntitlementSource, ref *string, until time.Time, _ string, reason string) (int64, error) {
	var n int64
	for _, r := range f.rows {
		if r.OrganizationID == org && r.Source == source && r.Status == model.EntitlementStatusActive && (ref == nil || r.SourceReference == *ref) {
			r.Status, r.EffectiveUntil, r.Reason = model.EntitlementStatusExpired, &until, reason
			n++
		}
	}
	return n, nil
}

func (f *fakeContractEntStore) ExpireByID(_ context.Context, id string, until time.Time, _ string, reason string) error {
	for _, r := range f.rows {
		if r.ID == id && r.Status == model.EntitlementStatusActive {
			r.Status, r.EffectiveUntil, r.Reason = model.EntitlementStatusExpired, &until, reason
		}
	}
	return nil
}

func (f *fakeContractEntStore) active(source model.EntitlementSource) []string {
	var keys []string
	for _, r := range f.rows {
		if r.Source == source && r.Status == model.EntitlementStatusActive {
			keys = append(keys, r.FeatureKey)
		}
	}
	sort.Strings(keys)
	return keys
}

func newContractEntitlementTest() (*ContractEntitlementService, *fakeContractEntStore) {
	store := &fakeContractEntStore{}
	svc := NewContractEntitlementService(store)
	svc.now = func() time.Time { return time.Date(2026, 10, 7, 3, 0, 0, 0, time.UTC) }
	return svc, store
}

func grant(key, valueType, value string) FeatureGrant {
	return FeatureGrant{Key: key, ValueType: valueType, Value: json.RawMessage(value)}
}

func TestLimitsFromValue(t *testing.T) {
	cases := []struct {
		valueType, value string
		want             map[string]any
	}{
		{"boolean", `true`, map[string]any{"value": true, "enabled": true}},
		{"boolean", `false`, map[string]any{"value": false, "enabled": false}},
		{"integer", `5`, map[string]any{"value": int64(5), "limit": int64(5)}},
		{"decimal", `"12.50"`, map[string]any{"value": "12.50"}},
		{"decimal", `12.5`, map[string]any{"value": "12.5"}},
		{"string", `"pro"`, map[string]any{"value": "pro"}},
	}
	for _, c := range cases {
		got, err := LimitsFromValue(c.valueType, json.RawMessage(c.value))
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("LimitsFromValue(%s, %s) = %v, %v; want %v", c.valueType, c.value, got, err, c.want)
		}
	}
	for _, bad := range [][2]string{{"integer", `5.5`}, {"integer", `"x"`}, {"boolean", `1`}, {"string", `3`}, {"nope", `1`}} {
		if _, err := LimitsFromValue(bad[0], json.RawMessage(bad[1])); err == nil {
			t.Errorf("LimitsFromValue(%s, %s) must fail", bad[0], bad[1])
		}
	}
}

func TestGrantContractExpiresDefault(t *testing.T) {
	svc, store := newContractEntitlementTest()
	ctx := context.Background()
	if err := svc.GrantDefault(ctx, "org1", []FeatureGrant{grant("user.max", "integer", `1`)}, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.GrantContract(ctx, "org1", "ctr1", "CTR-1", []FeatureGrant{grant("user.max", "integer", `5`)}, ""); err != nil {
		t.Fatal(err)
	}
	if got := store.active(model.EntitlementSourceDefault); len(got) != 0 {
		t.Fatalf("default still active: %v", got)
	}
	if got := store.active(model.EntitlementSourceContract); !reflect.DeepEqual(got, []string{"user.max"}) {
		t.Fatalf("contract active = %v", got)
	}
}

func TestGrantContractIdempotent(t *testing.T) {
	svc, store := newContractEntitlementTest()
	ctx := context.Background()
	grants := []FeatureGrant{grant("user.max", "integer", `5`), grant("crm.enabled", "boolean", `true`)}
	for i := 0; i < 2; i++ {
		if err := svc.GrantContract(ctx, "org1", "ctr1", "CTR-1", grants, ""); err != nil {
			t.Fatal(err)
		}
	}
	if len(store.rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(store.rows))
	}
	for _, r := range store.rows {
		if r.Version != 1 || r.Status != model.EntitlementStatusActive || r.SourceReference != "ctr1" {
			t.Fatalf("row %+v", *r)
		}
	}
	if store.upserts != 2 {
		t.Fatalf("upserts = %d, want 2 (second grant must be a no-op)", store.upserts)
	}
}

func TestGrantContractChangedValueKeepsEffectiveFrom(t *testing.T) {
	svc, store := newContractEntitlementTest()
	ctx := context.Background()
	_ = svc.GrantContract(ctx, "org1", "ctr1", "CTR-1", []FeatureGrant{grant("user.max", "integer", `5`)}, "")
	first := store.rows[0].EffectiveFrom
	svc.now = func() time.Time { return first.Add(time.Hour) }
	_ = svc.GrantContract(ctx, "org1", "ctr1", "CTR-1", []FeatureGrant{grant("user.max", "integer", `10`)}, "")
	if store.rows[0].Limits["limit"] != float64(10) || !store.rows[0].EffectiveFrom.Equal(first) {
		t.Fatalf("row = %+v", *store.rows[0])
	}
}

func TestGrantContractDropsRemovedFeature(t *testing.T) {
	svc, store := newContractEntitlementTest()
	ctx := context.Background()
	_ = svc.GrantContract(ctx, "org1", "ctr1", "CTR-1", []FeatureGrant{grant("user.max", "integer", `5`), grant("crm.enabled", "boolean", `true`)}, "")
	_ = svc.GrantContract(ctx, "org1", "ctr1", "CTR-1", []FeatureGrant{grant("user.max", "integer", `5`)}, "")
	if got := store.active(model.EntitlementSourceContract); !reflect.DeepEqual(got, []string{"user.max"}) {
		t.Fatalf("contract active = %v", got)
	}
}

func TestRevokeContractReportsRemaining(t *testing.T) {
	svc, store := newContractEntitlementTest()
	ctx := context.Background()
	_ = svc.GrantContract(ctx, "org1", "ctr1", "CTR-1", []FeatureGrant{grant("user.max", "integer", `5`)}, "")
	_ = svc.GrantContract(ctx, "org1", "ctr2", "CTR-2", []FeatureGrant{grant("crm.enabled", "boolean", `true`)}, "")
	remaining, err := svc.RevokeContract(ctx, "org1", "ctr1", "CTR-1", "")
	if err != nil || !remaining {
		t.Fatalf("remaining=%v err=%v, want true", remaining, err)
	}
	remaining, err = svc.RevokeContract(ctx, "org1", "ctr2", "CTR-2", "")
	if err != nil || remaining {
		t.Fatalf("remaining=%v err=%v, want false", remaining, err)
	}
	if got := store.active(model.EntitlementSourceContract); len(got) != 0 {
		t.Fatalf("contract active = %v", got)
	}
}

func TestGrantDefaultSkippedWhenContractActive(t *testing.T) {
	svc, store := newContractEntitlementTest()
	ctx := context.Background()
	_ = svc.GrantContract(ctx, "org1", "ctr1", "CTR-1", []FeatureGrant{grant("user.max", "integer", `5`)}, "")
	if err := svc.GrantDefault(ctx, "org1", []FeatureGrant{grant("user.max", "integer", `1`)}, ""); err != nil {
		t.Fatal(err)
	}
	if got := store.active(model.EntitlementSourceDefault); len(got) != 0 {
		t.Fatalf("default active = %v", got)
	}
	_, _ = svc.RevokeContract(ctx, "org1", "ctr1", "CTR-1", "")
	if err := svc.GrantDefault(ctx, "org1", []FeatureGrant{grant("user.max", "integer", `1`)}, ""); err != nil {
		t.Fatal(err)
	}
	if got := store.active(model.EntitlementSourceDefault); !reflect.DeepEqual(got, []string{"user.max"}) {
		t.Fatalf("default active = %v", got)
	}
}
