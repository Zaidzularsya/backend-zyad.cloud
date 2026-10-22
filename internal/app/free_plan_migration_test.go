package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

type fakeFreePlanStore struct {
	orgs      []FreePlanOrgRef
	contract  map[string]bool
	defaultBy map[string][]organizationmodel.Entitlement
}

func (f fakeFreePlanStore) ListDefaultOrganizations(context.Context, time.Time) ([]FreePlanOrgRef, error) {
	return f.orgs, nil
}
func (f fakeFreePlanStore) HasActiveContract(_ context.Context, orgID string, _ time.Time) (bool, error) {
	return f.contract[orgID], nil
}
func (f fakeFreePlanStore) ActiveDefault(_ context.Context, orgID string, _ time.Time) ([]organizationmodel.Entitlement, error) {
	return f.defaultBy[orgID], nil
}

func ent(key string, limits map[string]any) organizationmodel.Entitlement {
	return organizationmodel.Entitlement{FeatureKey: key, Source: organizationmodel.EntitlementSourceDefault, Limits: limits}
}

var testFreeDefs = map[string]catalogdomain.FeatureDef{
	"crm.enabled":    {Key: "crm.enabled", ValueType: "boolean"},
	"crm.max_leads":  {Key: "crm.max_leads", ValueType: "integer"},
	"landing.legacy": {Key: "landing.legacy", ValueType: "boolean"},
}

func testFreeFeatures() []catalogdomain.ProductFeature {
	return []catalogdomain.ProductFeature{
		{FeatureKey: "crm.enabled", Value: json.RawMessage(`true`)},
		{FeatureKey: "crm.max_leads", Value: json.RawMessage(`50`)},
		{FeatureKey: "not.in.registry", Value: json.RawMessage(`1`)},
	}
}

func TestPlanFreePlanReprovision(t *testing.T) {
	store := fakeFreePlanStore{
		orgs:     []FreePlanOrgRef{{ID: "a", Slug: "alpha"}, {ID: "b", Slug: "beta"}, {ID: "c", Slug: "gamma"}},
		contract: map[string]bool{"c": true},
		defaultBy: map[string][]organizationmodel.Entitlement{
			// alpha: crm.enabled false -> true, no max_leads yet, legacy key to be removed.
			"a": {ent("crm.enabled", map[string]any{"value": false, "enabled": false}), ent("landing.legacy", map[string]any{"value": true, "enabled": true})},
			// beta: already identical (numbers decoded as float64 from JSON).
			"b": {ent("crm.enabled", map[string]any{"value": true, "enabled": true}), ent("crm.max_leads", map[string]any{"value": float64(50), "limit": float64(50)})},
		},
	}
	plan, err := PlanFreePlanReprovision(context.Background(), store, testFreeDefs, testFreeFeatures(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Orgs) != 1 || plan.Orgs[0].Org.Slug != "alpha" {
		t.Fatalf("affected orgs = %+v", plan.Orgs)
	}
	if plan.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", plan.Unchanged)
	}
	if len(plan.SkippedContract) != 1 || plan.SkippedContract[0].Slug != "gamma" {
		t.Errorf("SkippedContract = %+v", plan.SkippedContract)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "not.in.registry") {
		t.Errorf("Warnings = %v", plan.Warnings)
	}
	changes := map[string]FeatureChange{}
	for _, c := range plan.Orgs[0].Changes {
		changes[c.Key] = c
	}
	if len(changes) != 3 {
		t.Fatalf("changes = %+v", plan.Orgs[0].Changes)
	}
	if c := changes["crm.enabled"]; c.Before["enabled"] != false || c.After["enabled"] != true {
		t.Errorf("crm.enabled change = %+v", c)
	}
	if c := changes["crm.max_leads"]; c.Before != nil || c.After["limit"] != int64(50) {
		t.Errorf("crm.max_leads change = %+v", c)
	}
	if c := changes["landing.legacy"]; c.Before == nil || c.After != nil {
		t.Errorf("landing.legacy change = %+v", c)
	}
}

func TestDiffProductFeatures(t *testing.T) {
	current := []catalogdomain.ProductFeature{
		{FeatureKey: "crm.enabled", Value: json.RawMessage(`false`)},
		{FeatureKey: "old.key", Value: json.RawMessage(`1`)},
		{FeatureKey: "crm.max_leads", Value: json.RawMessage(`50`)},
	}
	diff := DiffProductFeatures(current, testFreeFeatures()[:2])
	got := map[string]string{}
	for _, d := range diff {
		got[d.Key] = string(d.Before) + "->" + string(d.After)
	}
	want := map[string]string{"crm.enabled": "false->true", "old.key": "1->"}
	if len(got) != len(want) {
		t.Fatalf("diff = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

type fakeProvisioner struct{ called []string }

func (f *fakeProvisioner) ProvisionDefaultAccess(_ context.Context, orgID string) error {
	f.called = append(f.called, orgID)
	return nil
}

func TestApplyFreePlanProvisionsOnlyAffectedOrgs(t *testing.T) {
	plan := FreePlanPlan{Orgs: []OrgChange{{Org: FreePlanOrgRef{ID: "a"}}, {Org: FreePlanOrgRef{ID: "z"}}}}
	p := &fakeProvisioner{}
	n, err := ApplyFreePlan(context.Background(), plan, p)
	if err != nil || n != 2 || len(p.called) != 2 || p.called[0] != "a" || p.called[1] != "z" {
		t.Fatalf("n=%d err=%v called=%v", n, err, p.called)
	}
}

func TestWriteFreePlanReportDryRunSaysNothingWritten(t *testing.T) {
	var buf bytes.Buffer
	plan := FreePlanPlan{Orgs: []OrgChange{{Org: FreePlanOrgRef{ID: "a", Slug: "alpha"}, Changes: []FeatureChange{
		{Key: "crm.max_leads", After: map[string]any{"value": int64(50), "limit": int64(50)}},
	}}}}
	WriteFreePlanReport(&buf, nil, plan, false)
	out := buf.String()
	if !strings.Contains(out, "DRY-RUN") || !strings.Contains(out, "alpha") || !strings.Contains(out, "crm.max_leads") {
		t.Fatalf("report = %s", out)
	}
}
