//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestEntitlementSourcesAccepted(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("entsrc"), ".", "-")

	organization, err := NewOrganizationRepository(db).Create(ctx, CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "entsrc-" + suffix,
		Name:          "Entitlement Source Test",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM organization_entitlements WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, organization.ID)
	})

	for _, source := range []string{"contract", "default"} {
		if _, err := db.Exec(ctx, `
			INSERT INTO organization_entitlements (organization_id, feature_key, source, limits)
			VALUES ($1, 'user.max', $2, '{}'::jsonb)`, organization.ID, source); err != nil {
			t.Fatalf("source %q should be accepted: %v", source, err)
		}
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO organization_entitlements (organization_id, feature_key, source, limits)
		VALUES ($1, 'user.max', 'foo', '{}'::jsonb)`, organization.ID); err == nil {
		t.Fatal("source foo should be rejected")
	}
}

func TestFindEffectivePrefersContractOverPlanAndDefault(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("entprio"), ".", "-")
	repo := NewEntitlementRepository(db)

	organization, err := NewOrganizationRepository(db).Create(ctx, CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "entprio-" + suffix,
		Name:          "Entitlement Priority Test",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	var actorID string
	if err := db.QueryRow(ctx, `INSERT INTO users (name, email, status) VALUES ('Prio Actor', $1, 'active') RETURNING id`,
		"entprio-"+suffix+"@example.test").Scan(&actorID); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM organization_entitlements WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, organization.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, actorID)
	})

	now := time.Now().UTC().Truncate(time.Microsecond)
	from := now.Add(-time.Hour)
	put := func(source model.EntitlementSource, ref string, limit int, until *time.Time) {
		t.Helper()
		if _, err := repo.Upsert(ctx, UpsertEntitlementParams{
			OrganizationID: organization.ID, FeatureKey: "user.max", Source: source, SourceReference: ref,
			Limits: map[string]any{"value": limit, "limit": limit}, EffectiveFrom: from, EffectiveUntil: until,
			Reason: "priority test", ActorUserID: actorID,
		}); err != nil {
			t.Fatalf("upsert %s: %v", source, err)
		}
	}
	valueOf := func() (model.EntitlementSource, float64) {
		t.Helper()
		effective, err := repo.FindEffective(ctx, organization.ID, "user.max", now)
		if err != nil {
			t.Fatalf("FindEffective: %v", err)
		}
		list, _, err := repo.ListEffective(ctx, organization.ID, EntitlementListFilter{}, now)
		if err != nil || len(list) != 1 || list[0].Source != effective.Source {
			t.Fatalf("ListEffective = %+v err=%v, want same winner as FindEffective", list, err)
		}
		return effective.Source, effective.Limits["value"].(float64)
	}

	put(model.EntitlementSourcePlan, "sub-1", 10, nil)
	put(model.EntitlementSourceDefault, "", 1, nil)
	if source, value := valueOf(); source != model.EntitlementSourcePlan || value != 10 {
		t.Fatalf("plan vs default winner = %s/%v", source, value)
	}
	put(model.EntitlementSourceContract, "ctr-1", 50, nil)
	if source, value := valueOf(); source != model.EntitlementSourceContract || value != 50 {
		t.Fatalf("contract winner = %s/%v", source, value)
	}
	until := now.Add(time.Hour)
	put(model.EntitlementSourcePlatformOverride, "", 99, &until)
	if source, value := valueOf(); source != model.EntitlementSourcePlatformOverride || value != 99 {
		t.Fatalf("override winner = %s/%v", source, value)
	}
}

func TestEntitlementExpireAndActiveSourceHelpers(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("entexp"), ".", "-")
	repo := NewEntitlementRepository(db)

	organization, err := NewOrganizationRepository(db).Create(ctx, CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "entexp-" + suffix,
		Name:          "Entitlement Expire Test",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM organization_entitlements WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, organization.ID)
	})

	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, ref := range []string{"ctr-a", "ctr-b"} {
		if _, err := repo.Upsert(ctx, UpsertEntitlementParams{
			OrganizationID: organization.ID, FeatureKey: "user.max", Source: model.EntitlementSourceContract,
			SourceReference: ref, Limits: map[string]any{"value": 1}, EffectiveFrom: now.Add(-time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	has, err := repo.HasActiveSource(ctx, organization.ID, model.EntitlementSourceContract, now)
	if err != nil || !has {
		t.Fatalf("HasActiveSource = %v err=%v", has, err)
	}
	ref := "ctr-a"
	if n, err := repo.ExpireBySource(ctx, organization.ID, model.EntitlementSourceContract, &ref, now, "", "ended"); err != nil || n != 1 {
		t.Fatalf("ExpireBySource ctr-a = %d err=%v", n, err)
	}
	if n, err := repo.ExpireBySource(ctx, organization.ID, model.EntitlementSourceContract, &ref, now, "", "ended"); err != nil || n != 0 {
		t.Fatalf("second ExpireBySource = %d err=%v, want 0", n, err)
	}
	active, err := repo.ListActiveBySource(ctx, organization.ID, model.EntitlementSourceContract, nil, now)
	if err != nil || len(active) != 1 || active[0].SourceReference != "ctr-b" {
		t.Fatalf("active = %+v err=%v", active, err)
	}
	if err := repo.ExpireByID(ctx, active[0].ID, now, "", "feature removed"); err != nil {
		t.Fatal(err)
	}
	if has, _ := repo.HasActiveSource(ctx, organization.ID, model.EntitlementSourceContract, now); has {
		t.Fatal("no contract entitlement should remain active")
	}
}
