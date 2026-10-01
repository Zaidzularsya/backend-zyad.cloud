//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/core/businesstime"
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
			for _, table := range []string{
				"crm_playbook_runs", "crm_activities", "crm_leads", "crm_deals",
				"crm_contacts", "crm_companies", "crm_pipeline_stages", "crm_pipelines", "crm_settings",
			} {
				_, _ = tx.Exec(ctx, "DELETE FROM "+table+" WHERE organization_id = $1", orgID)
			}
			_ = tx.Commit(ctx)
		}
	})
}

func TestLeadEventsAndDashboardIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)

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

	// Legacy data: a note stored as pending must never count as a follow-up.
	if _, err := activities.Create(ctx, tenants.A.Scope, repository.CreateActivityParams{
		RelatedEntityType: domain.ActivityEntityLead, RelatedEntityID: kept.ID,
		Type: domain.ActivityTypeNote, Subject: "Catatan lama pending",
	}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	fixTx, _ := db.Begin(ctx)
	_, _ = fixTx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", tenants.A.OrganizationID)
	_, _ = fixTx.Exec(ctx, "UPDATE crm_activities SET status = 'pending', completed_at = NULL WHERE type = 'note' AND organization_id = $1", tenants.A.OrganizationID)
	_ = fixTx.Commit(ctx)

	today := businesstime.DayOf(time.Now())
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
	if len(d.UpcomingFollowUps) != 1 || d.UpcomingFollowUps[0].LeadName != "Andi" || d.FollowUps.Pending != 1 {
		t.Errorf("upcoming = %+v, want one follow-up on Andi", d.UpcomingFollowUps)
	}

	kinds := map[string]int{}
	for _, it := range d.RecentActivity {
		kinds[it.Kind]++
	}
	// Budi's created event is hidden once the lead is deleted but
	// its deleted event stays.
	// activity_created:2 = the follow-up call + the legacy note (notes are history, just not follow-ups).
	want := map[string]int{"created": 1, "status_changed": 1, "deleted": 1, "activity_created": 2}
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

	// List filters (Fase 2): source is case-insensitive, created range is
	// inclusive, sort keys map to whitelisted expressions.
	extra, err := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Citra", Source: "website"})
	if err != nil {
		t.Fatalf("Create extra: %v", err)
	}
	listed, total, err := leads.List(ctx, tenants.A.Scope, repository.LeadListFilter{
		Source: "WEBSITE", CreatedFrom: businesstime.DayStartUTC(today), CreatedToExclusive: businesstime.DayStartUTC(today.AddDate(0, 0, 1)), Sort: "contact_name",
	})
	if err != nil {
		t.Fatalf("List filtered: %v", err)
	}
	if total != 2 || len(listed) != 2 || listed[0].ContactName != "Andi" || listed[1].ID != extra.ID {
		t.Errorf("filtered list = %d %+v, want [Andi, Citra]", total, listed)
	}
	if _, total, err := leads.List(ctx, tenants.A.Scope, repository.LeadListFilter{CreatedToExclusive: businesstime.DayStartUTC(today)}); err != nil || total != 0 {
		t.Errorf("list before today = %d, %v; want 0", total, err)
	}

	other, err := dashboards.Dashboard(ctx, tenants.B.Scope, rng, limits)
	if err != nil {
		t.Fatalf("Dashboard B: %v", err)
	}
	if other.Created != 0 || len(other.RecentActivity) != 0 || other.FollowUps.Pending != 0 {
		t.Errorf("tenant B sees tenant A data: %+v", other)
	}
}

// Timestamps are stored as UTC wall-clock, but days are Asia/Jakarta days:
// a lead created at 23:30 WIB and one at 00:30 WIB belong to different
// buckets, and "today" for follow-ups flips at 00:00 WIB (17:00Z), not at UTC
// midnight.
func TestLeadDashboardUsesBusinessDaysIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)
	orgID := tenants.A.OrganizationID

	leads := repository.NewLeadRepository(db)
	activities := repository.NewActivityRepository(db)

	setUTC := func(table, column, id string, at time.Time) {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID); err != nil {
			t.Fatalf("set org: %v", err)
		}
		if _, err := tx.Exec(ctx, "UPDATE "+table+" SET "+column+" = $2 WHERE id = $1", id, at); err != nil {
			t.Fatalf("set %s.%s: %v", table, column, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	late, err := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Malam", Source: "web"})
	if err != nil {
		t.Fatalf("Create late: %v", err)
	}
	early, err := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Dini", Source: "web"})
	if err != nil {
		t.Fatalf("Create early: %v", err)
	}
	setUTC("crm_leads", "created_at", late.ID, time.Date(2026, 1, 10, 16, 30, 0, 0, time.UTC))  // 23:30 WIB, 10 Jan
	setUTC("crm_leads", "created_at", early.ID, time.Date(2026, 1, 10, 17, 30, 0, 0, time.UTC)) // 00:30 WIB, 11 Jan

	mkActivity := func(subject string, due time.Time) {
		t.Helper()
		if _, err := activities.Create(ctx, tenants.A.Scope, repository.CreateActivityParams{
			RelatedEntityType: domain.ActivityEntityLead, RelatedEntityID: late.ID,
			Type: domain.ActivityTypeCall, Subject: subject, DueAt: &due,
		}); err != nil {
			t.Fatalf("Create activity %s: %v", subject, err)
		}
	}
	mkActivity("kemarin", time.Date(2026, 1, 9, 16, 59, 0, 0, time.UTC))     // 23:59 WIB, 9 Jan  -> overdue
	mkActivity("hari ini", time.Date(2026, 1, 10, 16, 0, 0, 0, time.UTC))    // 23:00 WIB, 10 Jan -> today
	mkActivity("besok dini", time.Date(2026, 1, 10, 17, 30, 0, 0, time.UTC)) // 00:30 WIB, 11 Jan -> next 7 days

	day := func(d int) time.Time { return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC) }
	rng := domain.LeadDashboardRange{
		From: day(10), To: day(11), PreviousFrom: day(8), PreviousTo: day(9),
		Granularity: domain.LeadDashboardDaily,
	}
	nowWIB := time.Date(2026, 1, 10, 16, 30, 0, 0, time.UTC) // 23:30 WIB, 10 Jan
	dashboards := repository.NewLeadDashboardRepositoryWithClock(db, func() time.Time { return nowWIB })

	d, err := dashboards.Dashboard(ctx, tenants.A.Scope, rng, repository.LeadDashboardLimits{FollowUps: 8, RecentActivity: 10})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}

	if len(d.Series) != 2 {
		t.Fatalf("series = %+v, want 2 daily buckets", d.Series)
	}
	for i, wantDay := range []time.Time{day(10), day(11)} {
		if !d.Series[i].Bucket.Equal(wantDay) || d.Series[i].Created != 1 {
			t.Errorf("bucket[%d] = %+v, want %s created=1", i, d.Series[i], wantDay.Format("2006-01-02"))
		}
	}
	if d.Created != 2 || d.PreviousCreated != 0 {
		t.Errorf("created = %d (prev %d), want 2 (prev 0)", d.Created, d.PreviousCreated)
	}
	if f := d.FollowUps; f.Pending != 3 || f.Overdue != 1 || f.DueToday != 1 || f.DueNext7Days != 1 {
		t.Errorf("follow-ups = %+v, want pending=3 overdue=1 today=1 next7=1", f)
	}

	// A lead created 00:30 WIB on 11 Jan is out of a "10 Jan only" range.
	one := rng
	one.To = day(10)
	d, err = dashboards.Dashboard(ctx, tenants.A.Scope, one, repository.LeadDashboardLimits{FollowUps: 8, RecentActivity: 10})
	if err != nil {
		t.Fatalf("Dashboard single day: %v", err)
	}
	if d.Created != 1 {
		t.Errorf("created on 10 Jan only = %d, want 1", d.Created)
	}
}

func TestDashboardShowsPlaybookStepName(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	leads := repository.NewLeadRepository(db)
	lead, _ := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Dash"})
	attempting := domain.LeadStatusAttempting
	_, _ = leads.Update(ctx, tenants.A.Scope, lead.ID, repository.UpdateLeadParams{Status: &attempting})

	today := businesstime.DayOf(time.Now())
	d, err := repository.NewLeadDashboardRepository(db).Dashboard(ctx, tenants.A.Scope, domain.LeadDashboardRange{
		From: today.AddDate(0, 0, -6), To: today,
		PreviousFrom: today.AddDate(0, 0, -13), PreviousTo: today.AddDate(0, 0, -7),
		Granularity: domain.LeadDashboardDaily,
	}, repository.LeadDashboardLimits{FollowUps: 8, RecentActivity: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.UpcomingFollowUps) != 1 || d.UpcomingFollowUps[0].StepName != "Kontak pertama" {
		t.Fatalf("follow-ups %+v", d.UpcomingFollowUps)
	}
	if pb := d.UpcomingFollowUps[0].Activity.Playbook; pb == nil || len(pb.Outcomes) == 0 {
		t.Fatalf("follow-up must carry playbook outcomes, got %+v", pb)
	}
	if d.StatusCounts[domain.LeadStatusAttempting] != 1 {
		t.Fatalf("status counts %+v", d.StatusCounts)
	}
}
