//go:build integration

package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestRevisionRepositoryIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	_, err := db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES 
		('11111111-1111-1111-1111-111111111111', 'Mock User A', 'mock_a@example.com', 'active'),
		('22222222-2222-2222-2222-222222222222', 'Mock User B', 'mock_b@example.com', 'active')
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		t.Fatalf("failed to insert mock users: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES 
		($1, 'customer', 'organization-a', 'Organization A', 'active'),
		($2, 'customer', 'organization-b', 'Organization B', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID, tenants.B.OrganizationID)
	if err != nil {
		t.Fatalf("failed to insert mock organizations: %v", err)
	}

	pageRepo := repository.NewPageRepository(db)
	revisionRepo := repository.NewRevisionRepository(db)

	slugA := strings.ReplaceAll("page-"+testutil.UniqueCode("revisionrepoa"), ".", "-")

	// Create Page
	pageA, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Page A",
		Title:      "Campaign A",
		Slug:       slugA,
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
		CreatedBy:  "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create page for Tenant A: %v", err)
	}

	// --- 1. Revisions ---
	rev1, err := revisionRepo.CreateRevision(ctx, tenants.A.Scope, repository.CreateRevisionParams{
		LandingPageID:  pageA.ID,
		RevisionNumber: 1,
		Snapshot:       map[string]any{"content": "v1"},
		ChangeNote:     "First draft",
		CreatedBy:      "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create revision 1: %v", err)
	}

	rev2, err := revisionRepo.CreateRevision(ctx, tenants.A.Scope, repository.CreateRevisionParams{
		LandingPageID:  pageA.ID,
		RevisionNumber: 2,
		Snapshot:       map[string]any{"content": "v2"},
		ChangeNote:     "Second draft",
		CreatedBy:      "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create revision 2: %v", err)
	}

	_, err = revisionRepo.GetRevision(ctx, tenants.B.Scope, rev1.ID)
	if err == nil {
		t.Error("Expected Tenant B to fail reading Tenant A's revision")
	}

	latestRev, err := revisionRepo.GetLatestRevision(ctx, tenants.A.Scope, pageA.ID)
	if err != nil {
		t.Fatalf("Get latest revision: %v", err)
	}
	if latestRev.ID != rev2.ID {
		t.Errorf("Expected latest revision to be rev2, got %s", latestRev.ID)
	}

	// --- 2. Schedules ---
	schedA, err := revisionRepo.CreateSchedule(ctx, tenants.A.Scope, repository.CreateScheduleParams{
		LandingPageID: pageA.ID,
		Action:        domain.ScheduleActionPublish,
		ScheduledAt:   time.Now().Add(-24 * time.Hour), // Past due for claiming
		CreatedBy:     "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create schedule: %v", err)
	}

	schedules, err := revisionRepo.ListSchedules(ctx, tenants.A.Scope, pageA.ID)
	if err != nil {
		t.Fatalf("List schedules: %v", err)
	}
	if len(schedules) != 1 {
		t.Fatalf("Expected 1 schedule, got %d", len(schedules))
	}

	// 3. Claim Pending Schedules (Worker operation)
	claimed, err := revisionRepo.ClaimPendingSchedules(ctx, tenants.A.Scope, 10, 5*time.Minute)
	if err != nil {
		t.Fatalf("Claim pending schedules: %v", err)
	}

	var foundClaim *domain.LandingPageSchedule
	for _, c := range claimed {
		if c.ID == schedA.ID {
			foundClaim = &c
			break
		}
	}
	if foundClaim == nil {
		t.Fatalf("Expected schedA %s to be claimed, but wasn't. Total claimed: %d", schedA.ID, len(claimed))
	}
	if foundClaim.Status != domain.ScheduleStatusProcessing {
		t.Errorf("Expected status 'processing', got %s", foundClaim.Status)
	}
	if foundClaim.LockID == nil {
		t.Error("Expected lock_id to be populated")
	}

	// 4. Mark Schedule Status (Worker operation)
	errMsg := "Simulated error"
	err = revisionRepo.MarkScheduleStatus(ctx, tenants.A.Scope, foundClaim.ID, domain.ScheduleStatusFailed, &errMsg)
	if err != nil {
		t.Fatalf("Mark schedule status: %v", err)
	}

	// Verify update
	updatedSched, err := revisionRepo.GetSchedule(ctx, tenants.A.Scope, claimed[0].ID)
	if err != nil {
		t.Fatalf("Get updated schedule: %v", err)
	}
	if updatedSched.Status != domain.ScheduleStatusFailed {
		t.Errorf("Expected status 'failed', got %s", updatedSched.Status)
	}
	if updatedSched.ErrorMessage != errMsg {
		t.Errorf("Expected error message '%s', got '%s'", errMsg, updatedSched.ErrorMessage)
	}
	if updatedSched.LockID != nil {
		t.Error("Expected lock_id to be null after completion/failure")
	}

	// 5. Delete pending schedule (Create a new one first)
	schedB, err := revisionRepo.CreateSchedule(ctx, tenants.A.Scope, repository.CreateScheduleParams{
		LandingPageID: pageA.ID,
		Action:        domain.ScheduleActionUnpublish,
		ScheduledAt:   time.Now().UTC().Add(1 * time.Hour), // Future
		CreatedBy:     "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create second schedule: %v", err)
	}

	err = revisionRepo.DeleteSchedule(ctx, tenants.A.Scope, schedB.ID)
	if err != nil {
		t.Fatalf("Delete schedule: %v", err)
	}
}
