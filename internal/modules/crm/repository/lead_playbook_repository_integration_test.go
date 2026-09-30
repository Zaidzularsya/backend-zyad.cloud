//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

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

func pendingPlaybookSteps(t *testing.T, db *database.Pool, scope coretenant.Scope, leadID string) []domain.Activity {
	t.Helper()
	list, _, err := repository.NewActivityRepository(db).List(context.Background(), scope, repository.ActivityListFilter{
		RelatedEntityType: domain.ActivityEntityLead, RelatedEntityID: leadID, Status: domain.ActivityStatusPending,
	})
	if err != nil {
		t.Fatalf("list activities: %v", err)
	}
	var out []domain.Activity
	for _, a := range list {
		if a.Playbook != nil {
			out = append(out, a)
		}
	}
	return out
}

func TestPlaybookStartsOnLeadCreate(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	creator := newCRMUser(t, db)

	lead, err := repository.NewLeadRepository(db).Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Andi", CreatedBy: creator})
	if err != nil {
		t.Fatal(err)
	}
	steps := pendingPlaybookSteps(t, db, tenants.A.Scope, lead.ID)
	if len(steps) != 1 || steps[0].Playbook.StepKey != "first_contact" || steps[0].AssigneeUserID != creator || steps[0].DueAt == nil {
		t.Fatalf("steps = %+v", steps)
	}
	if len(steps[0].Playbook.Outcomes) != 4 {
		t.Fatalf("first_contact must expose 4 normal outcomes, got %d", len(steps[0].Playbook.Outcomes))
	}
}

func TestPlaybookNotStartedWhenDisabled(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)

	lead, _ := repository.NewLeadRepository(db).Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Budi"})
	if steps := pendingPlaybookSteps(t, db, tenants.A.Scope, lead.ID); len(steps) != 0 {
		t.Fatalf("disabled org got %d steps", len(steps))
	}
}

func TestRunLifecycleOnLeadChanges(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	leads := repository.NewLeadRepository(db)
	owner2 := newCRMUser(t, db)

	// Owner change moves the pending step (R7).
	l1, _ := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Owner"})
	if _, err := leads.Assign(ctx, tenants.A.Scope, l1.ID, owner2, ""); err != nil {
		t.Fatal(err)
	}
	if s := pendingPlaybookSteps(t, db, tenants.A.Scope, l1.ID); len(s) != 1 || s[0].AssigneeUserID != owner2 {
		t.Fatalf("reassign: %+v", s)
	}

	// Manual status jump cancels the run (R6).
	l2, _ := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Jump"})
	qualified := domain.LeadStatusQualified
	if _, err := leads.Update(ctx, tenants.A.Scope, l2.ID, repository.UpdateLeadParams{Status: &qualified}); err != nil {
		t.Fatal(err)
	}
	if s := pendingPlaybookSteps(t, db, tenants.A.Scope, l2.ID); len(s) != 0 {
		t.Fatalf("status jump left %d steps", len(s))
	}

	// Status change that the current step allows keeps the run.
	l3, _ := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Keep"})
	attempting := domain.LeadStatusAttempting
	if _, err := leads.Update(ctx, tenants.A.Scope, l3.ID, repository.UpdateLeadParams{Status: &attempting}); err != nil {
		t.Fatal(err)
	}
	if s := pendingPlaybookSteps(t, db, tenants.A.Scope, l3.ID); len(s) != 1 {
		t.Fatalf("allowed status change must keep run, got %d", len(s))
	}

	// Delete cancels; restore does not revive (R6).
	l4, _ := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Del"})
	if err := leads.Delete(ctx, tenants.A.Scope, l4.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := leads.Restore(ctx, tenants.A.Scope, l4.ID, ""); err != nil {
		t.Fatal(err)
	}
	if s := pendingPlaybookSteps(t, db, tenants.A.Scope, l4.ID); len(s) != 0 {
		t.Fatalf("restore revived %d steps", len(s))
	}

	// Disqualify cancels and stores reason.
	l5, _ := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "DQ"})
	got, err := leads.Disqualify(ctx, tenants.A.Scope, l5.ID, repository.DisqualifyLeadParams{Reason: domain.DisqualifyDuplicate, Note: "sama dengan Andi"})
	if err != nil || got.Status != domain.LeadStatusUnqualified || got.DisqualifyReason != "duplicate" {
		t.Fatalf("disqualify %+v err=%v", got, err)
	}
	if s := pendingPlaybookSteps(t, db, tenants.A.Scope, l5.ID); len(s) != 0 {
		t.Fatalf("disqualify left %d steps", len(s))
	}

	// Manual start: 409 when active, works after cancel, not applicable for qualified.
	if _, err := leads.StartPlaybook(ctx, tenants.A.Scope, l3.ID, ""); !errors.Is(err, repository.ErrPlaybookAlreadyActive) {
		t.Fatalf("start active: %v", err)
	}
	if _, err := leads.StartPlaybook(ctx, tenants.A.Scope, l2.ID, ""); !errors.Is(err, repository.ErrPlaybookNotApplicable) {
		t.Fatalf("start qualified: %v", err)
	}
	contacted := domain.LeadStatusContacted
	if _, err := leads.Update(ctx, tenants.A.Scope, l2.ID, repository.UpdateLeadParams{Status: &contacted}); err != nil {
		t.Fatal(err)
	}
	if _, err := leads.StartPlaybook(ctx, tenants.A.Scope, l2.ID, ""); err != nil {
		t.Fatalf("start contacted: %v", err)
	}
	if s := pendingPlaybookSteps(t, db, tenants.A.Scope, l2.ID); len(s) != 1 || s[0].Playbook.StepKey != "discovery" {
		t.Fatalf("manual start step: %+v", s)
	}

	// Summary read model.
	sums, err := leads.FindPlaybookSummaries(ctx, tenants.A.Scope, []string{l2.ID, l5.ID})
	if err != nil || sums[l2.ID].StepKey != "discovery" || sums[l2.ID].Status != domain.PlaybookRunActive ||
		sums[l5.ID].Status != domain.PlaybookRunCompleted || *sums[l5.ID].Result != domain.PlaybookResultDisqualified {
		t.Fatalf("summaries %+v err=%v", sums, err)
	}
}

// CRMGateway.CreateLead calls LeadRepository.Create directly; this pins
// that the repository — not the lead service — starts the playbook.
func TestPlaybookStartsForWhatsAppCreatedLead(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	sessionOwner := newCRMUser(t, db)

	lead, err := repository.NewLeadRepository(db).Create(ctx, tenants.A.Scope, repository.CreateLeadParams{
		ContactName: "WhatsApp +6281234", Phone: "+6281234", Source: "whatsapp", OwnerUserID: sessionOwner, CreatedBy: sessionOwner,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := pendingPlaybookSteps(t, db, tenants.A.Scope, lead.ID)
	if len(s) != 1 || s[0].AssigneeUserID != sessionOwner {
		t.Fatalf("wa lead steps %+v", s)
	}
}

// newCRMUser inserts a throwaway user (owner/assignee FKs need a real row).
func newCRMUser(t *testing.T, db *database.Pool) string {
	t.Helper()
	var id string
	err := db.QueryRow(context.Background(), `
		INSERT INTO users (name, email, status, created_at, updated_at)
		VALUES ('Playbook Test', $1, 'active', now(), now()) RETURNING id::text`,
		fmt.Sprintf("playbook-%d@test.local", time.Now().UnixNano())).Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id) })
	return id
}
