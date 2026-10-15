//go:build integration

package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	crmrepo "zyad.cloud/internal/modules/crm/repository"
	crmservice "zyad.cloud/internal/modules/crm/service"
	landingdomain "zyad.cloud/internal/modules/landing/domain"
	landingrepo "zyad.cloud/internal/modules/landing/repository"
	landingservice "zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/platform/database/testutil"
)

// Real DB (TEST_DB_*): a platform-org form submission becomes a landing_page
// lead with a playbook run, and a repeat with the same email is attached as a
// note instead of a second lead. The fixture org is a regular test tenant; the
// platform path is exercised through isPlatformOrg=true (the feature gate is
// bypassed exactly as the handler does for platform organizations).
func TestLandingLeadSink_Integration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	scope := tenants.A.Scope
	orgID := tenants.A.OrganizationID

	if _, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'organization-a', 'Organization A', 'active')
		ON CONFLICT DO NOTHING`, orgID); err != nil {
		t.Fatalf("insert organization: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var ownerID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, status) VALUES ('Lead Sink Owner', $1, 'active') RETURNING id::text`,
		"lead-sink-"+suffix+"@test.local").Scan(&ownerID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	inOrgTx := func(fn func(exec func(sql string, args ...any))) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback(ctx)
		_, _ = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID)
		fn(func(sql string, args ...any) { _, _ = tx.Exec(ctx, sql, args...) })
		_ = tx.Commit(ctx)
	}
	// pageID/formID are filled once created; cleanup removes only this test's
	// landing rows because other packages share org A in the test DB.
	var pageID, formID string
	t.Cleanup(func() {
		inOrgTx(func(exec func(string, ...any)) {
			for _, table := range []string{"crm_playbook_runs", "crm_activities", "crm_leads", "crm_contacts", "crm_companies"} {
				exec("DELETE FROM "+table+" WHERE organization_id = $1", orgID)
			}
			exec("DELETE FROM landing_submissions WHERE organization_id = $1 AND form_id::text = $2", orgID, formID)
			exec("DELETE FROM landing_forms WHERE organization_id = $1 AND id::text = $2", orgID, formID)
			exec("DELETE FROM landing_pages WHERE organization_id = $1 AND id::text = $2", orgID, pageID)
		})
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id = $1::uuid`, ownerID)
	})

	pageRepo := landingrepo.NewPageRepository(db)
	formRepo := landingrepo.NewFormRepository(db)
	subRepo := landingrepo.NewSubmissionRepository(db)

	page, err := pageRepo.Create(ctx, scope, landingrepo.CreatePageParams{
		Name: "Sink Page", Title: "Sink Page", Slug: "sink-page-" + suffix,
		Type: landingdomain.PageTypeCampaign, Status: landingdomain.PageStatusDraft,
		Visibility: landingdomain.PageVisibilityPublic, Locale: "id-ID", Timezone: "Asia/Jakarta",
		CreatedBy: ownerID,
	})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	pageID = page.ID
	form, err := formRepo.Create(ctx, scope, landingrepo.CreateFormParams{
		LandingPageID: page.ID, Name: "Contact", Key: "contact-" + suffix, IsActive: true,
		CreatedBy: ownerID, CreateCRMLead: true, LeadOwnerUserID: ownerID,
	})
	if err != nil {
		t.Fatalf("create form: %v", err)
	}
	formID = form.ID

	leadRepo := crmrepo.NewLeadRepository(db)
	leadSvc := crmservice.NewLeadService(leadRepo, crmrepo.NewContactRepository(db), crmrepo.NewCompanyRepository(db))
	activitySvc := crmservice.NewActivityService(crmrepo.NewActivityRepository(db), leadRepo,
		crmrepo.NewContactRepository(db), crmrepo.NewCompanyRepository(db), crmrepo.NewDealRepository(db))
	sink := landingLeadSink{intake: crmservice.NewFormLeadIntake(leadRepo, leadSvc, activitySvc)}
	svc := landingservice.NewSubmissionService(subRepo, formRepo, pageRepo, sink, nil)

	email := "visitor-" + suffix + "@example.com"
	submit := func(key string) landingdomain.LandingSubmission {
		t.Helper()
		sub, err := svc.SubmitForm(ctx, scope, landingrepo.CreateSubmissionParams{
			FormID:         form.ID,
			Status:         landingdomain.SubmissionStatusNew,
			SubmittedData:  map[string]any{"name": "Visitor", "email": strings.ToUpper(email[:1]) + email[1:]},
			IdempotencyKey: key,
		}, true)
		if err != nil {
			t.Fatalf("SubmitForm: %v", err)
		}
		return sub
	}
	countRows := func(sql string, args ...any) int {
		t.Helper()
		var n int
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		_, _ = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID)
		if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}
	leadCount := func() int {
		return countRows(`SELECT count(*) FROM crm_leads WHERE organization_id = $1 AND lower(email) = lower($2) AND deleted_at IS NULL`, orgID, email)
	}

	first := submit("sink-key-1")
	if first.CRMSyncStatus != landingdomain.CRMSyncCreated || first.CRMLeadID == "" {
		t.Fatalf("first submission sync = %q lead=%q err=%q", first.CRMSyncStatus, first.CRMLeadID, first.CRMSyncError)
	}
	var source string
	var leadOwner string
	inTx := func(sql string, dest ...any) {
		tx, _ := db.Begin(ctx)
		defer tx.Rollback(ctx)
		_, _ = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID)
		if err := tx.QueryRow(ctx, sql, first.CRMLeadID).Scan(dest...); err != nil {
			t.Fatalf("query lead: %v", err)
		}
	}
	inTx(`SELECT source, owner_user_id::text FROM crm_leads WHERE id = $1`, &source, &leadOwner)
	if source != "landing_page" || leadOwner != ownerID {
		t.Fatalf("lead source=%q owner=%q", source, leadOwner)
	}
	if n := countRows(`SELECT count(*) FROM crm_playbook_runs WHERE entity_type = 'lead' AND entity_id = $1`, first.CRMLeadID); n != 1 {
		t.Fatalf("playbook runs = %d, want 1", n)
	}
	if n := leadCount(); n != 1 {
		t.Fatalf("lead count after first submit = %d", n)
	}

	second := submit("sink-key-2")
	if second.CRMSyncStatus != landingdomain.CRMSyncMerged || second.CRMLeadID != first.CRMLeadID {
		t.Fatalf("second submission sync = %q lead=%q (want merged into %q) err=%q",
			second.CRMSyncStatus, second.CRMLeadID, first.CRMLeadID, second.CRMSyncError)
	}
	if n := leadCount(); n != 1 {
		t.Fatalf("lead count after repeat submit = %d, want 1", n)
	}
	if n := countRows(`SELECT count(*) FROM crm_activities WHERE related_entity_type = 'lead' AND related_entity_id = $1 AND type = 'note' AND subject = 'Mengisi form lagi'`, first.CRMLeadID); n != 1 {
		t.Fatalf("note activities = %d, want 1", n)
	}
}
