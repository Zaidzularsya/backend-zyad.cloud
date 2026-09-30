//go:build integration

package repository_test

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestDefaultLeadPlaybookSeeded(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()

	var steps, outcomes int
	err := db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM crm_playbook_steps s JOIN crm_playbooks p ON p.id = s.playbook_id
				WHERE p.key = 'default_lead_sop' AND p.organization_id IS NULL),
			(SELECT COUNT(*) FROM crm_playbook_outcomes o
				JOIN crm_playbook_steps s ON s.id = o.step_id
				JOIN crm_playbooks p ON p.id = s.playbook_id
				WHERE p.key = 'default_lead_sop' AND p.organization_id IS NULL)`).Scan(&steps, &outcomes)
	if err != nil {
		t.Fatalf("query seed: %v", err)
	}
	if steps != 2 || outcomes != 8 {
		t.Fatalf("steps=%d outcomes=%d, want 2 and 8", steps, outcomes)
	}
}

func TestCRMSettingsDefaultsToEnabled(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)

	settings := repository.NewCRMSettingsRepository(db)
	got, err := settings.Get(ctx, tenants.A.Scope)
	if err != nil || !got.LeadPlaybookEnabled {
		t.Fatalf("default: %+v err=%v", got, err)
	}
	if _, err := settings.Update(ctx, tenants.A.Scope, false, ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = settings.Get(ctx, tenants.A.Scope)
	if got.LeadPlaybookEnabled {
		t.Fatal("A must be disabled")
	}
	other, _ := settings.Get(ctx, tenants.B.Scope)
	if !other.LeadPlaybookEnabled {
		t.Fatal("B must stay enabled (tenant isolation)")
	}
}

func TestCreateCompletedActivity(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)

	lead, err := repository.NewLeadRepository(db).Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Citra"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := repository.NewActivityRepository(db).Create(ctx, tenants.A.Scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityLead, RelatedEntityID: lead.ID,
		Type: domain.ActivityTypeNote, Subject: "Catatan", Status: domain.ActivityStatusCompleted,
	})
	if err != nil || a.Status != domain.ActivityStatusCompleted || a.CompletedAt == nil {
		t.Fatalf("got %+v err=%v", a, err)
	}
}

func disablePlaybook(t *testing.T, db *database.Pool, scope coretenant.Scope) {
	t.Helper()
	if _, err := repository.NewCRMSettingsRepository(db).Update(context.Background(), scope, false, ""); err != nil {
		t.Fatalf("disable playbook: %v", err)
	}
}
