//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

func setupCRMOrganizations(t *testing.T, db *database.Pool, tenants testutil.TenantPair) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES
		($1, 'customer', 'organization-a', 'Organization A', 'active'),
		($2, 'customer', 'organization-b', 'Organization B', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID, tenants.B.OrganizationID)
	if err != nil {
		t.Fatalf("insert organizations: %v", err)
	}
	t.Cleanup(func() {
		// crm_* is under FORCE RLS; delete per organization scope. Lead
		// events cascade from crm_leads.
		for _, orgID := range []string{tenants.A.OrganizationID, tenants.B.OrganizationID} {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Errorf("cleanup begin: %v", err)
				return
			}
			_, _ = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID)
			_, _ = tx.Exec(ctx, "DELETE FROM crm_activities WHERE organization_id = $1", orgID)
			_, _ = tx.Exec(ctx, "DELETE FROM crm_leads WHERE organization_id = $1", orgID)
			_ = tx.Commit(ctx)
		}
	})
}

func TestLeadEventsAndDashboardIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)

	leads := repository.NewLeadRepository(db)
	activities := repository.NewActivityRepository(db)
	dashboards := repository.NewLeadDashboardRepository(db)

	kept, err := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Andi", Source: "Website"})
	if err != nil {
		t.Fatalf("Create kept: %v", err)
	}
	removed, err := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Budi", Source: "referral"})
	if err != nil {
		t.Fatalf("Create removed: %v", err)
	}
	contacted := domain.LeadStatusContacted
	if _, err := leads.Update(ctx, tenants.A.Scope, kept.ID, repository.UpdateLeadParams{Status: &contacted}); err != nil {
		t.Fatalf("Update status: %v", err)
	}
	// Same status again must not record a second status_changed event.
	if _, err := leads.Update(ctx, tenants.A.Scope, kept.ID, repository.UpdateLeadParams{Status: &contacted}); err != nil {
		t.Fatalf("Update same status: %v", err)
	}
	if err := leads.Delete(ctx, tenants.A.Scope, removed.ID, ""); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	yesterday := time.Now().Add(-24 * time.Hour)
	if _, err := activities.Create(ctx, tenants.A.Scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityLead, RelatedEntityID: kept.ID,
		Type: domain.ActivityTypeCall, Subject: "Konfirmasi kebutuhan", DueAt: &yesterday,
	}); err != nil {
		t.Fatalf("Create activity: %v", err)
	}
	// Pending activity on the deleted lead must not count.
	if _, err := activities.Create(ctx, tenants.A.Scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityLead, RelatedEntityID: removed.ID,
		Type: domain.ActivityTypeEmail, Subject: "Orphan", DueAt: &yesterday,
	}); err != nil {
		t.Fatalf("Create activity on deleted lead: %v", err)
	}

	loc := time.FixedZone("WIB", 7*60*60)
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	rng := domain.LeadDashboardRange{
		From: today.AddDate(0, 0, -6), To: today,
		PreviousFrom: today.AddDate(0, 0, -13), PreviousTo: today.AddDate(0, 0, -7),
		Granularity: domain.LeadDashboardDaily,
	}
	limits := repository.LeadDashboardLimits{FollowUps: 8, RecentActivity: 10}

	d, err := dashboards.Dashboard(ctx, tenants.A.Scope, rng, limits)
	if err != nil {
		t.Fatalf("Dashboard A: %v", err)
	}

	if d.Created != 1 || d.PreviousCreated != 0 || d.Converted != 0 {
		t.Errorf("created/converted = %d/%d (prev %d), want 1/0 (prev 0)", d.Created, d.Converted, d.PreviousCreated)
	}
	if d.StatusCounts[domain.LeadStatusContacted] != 1 || d.StatusCounts[domain.LeadStatusNew] != 0 {
		t.Errorf("status counts = %v, want contacted=1 only", d.StatusCounts)
	}
	if d.StatusEntered[domain.LeadStatusNew] != 1 || d.StatusEntered[domain.LeadStatusContacted] != 1 {
		t.Errorf("status entered = %v, want new=1 contacted=1", d.StatusEntered)
	}
	if len(d.Series) != 7 {
		t.Fatalf("series len = %d, want 7 daily buckets", len(d.Series))
	}
	last := d.Series[len(d.Series)-1]
	if !last.Bucket.Equal(today) || last.Created != 1 {
		t.Errorf("last bucket = %+v, want %s created=1", last, today.Format("2006-01-02"))
	}
	if len(d.BySource) != 1 || d.BySource[0].Source != "website" || d.BySource[0].Count != 1 {
		t.Errorf("by source = %+v, want [website:1]", d.BySource)
	}
	if d.FollowUps.Pending != 1 || d.FollowUps.Overdue != 1 {
		t.Errorf("follow-up summary = %+v, want pending=1 overdue=1", d.FollowUps)
	}
	if len(d.UpcomingFollowUps) != 1 || d.UpcomingFollowUps[0].LeadName != "Andi" {
		t.Errorf("upcoming = %+v, want one follow-up on Andi", d.UpcomingFollowUps)
	}

	kinds := map[string]int{}
	for _, it := range d.RecentActivity {
		kinds[it.Kind]++
	}
	// Budi's created event is hidden once the lead is deleted but
	// its deleted event stays.
	want := map[string]int{"created": 1, "status_changed": 1, "deleted": 1, "activity_created": 1}
	for k, n := range want {
		if kinds[k] != n {
			t.Errorf("recent activity kinds = %v, want %v", kinds, want)
			break
		}
	}
	for i := 1; i < len(d.RecentActivity); i++ {
		if d.RecentActivity[i].OccurredAt.After(d.RecentActivity[i-1].OccurredAt) {
			t.Errorf("recent activity not sorted newest first at %d", i)
		}
	}

	other, err := dashboards.Dashboard(ctx, tenants.B.Scope, rng, limits)
	if err != nil {
		t.Fatalf("Dashboard B: %v", err)
	}
	if other.Created != 0 || len(other.RecentActivity) != 0 || other.FollowUps.Pending != 0 {
		t.Errorf("tenant B sees tenant A data: %+v", other)
	}
}
