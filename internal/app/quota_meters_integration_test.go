//go:build integration

package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

// meterFixture creates a dedicated organization per test so counts are not
// disturbed by other packages that share the fixed A/B test organizations.
func meterFixture(t *testing.T) (*database.Pool, coretenant.Scope, *organizationservice.QuotaMeterRegistry) {
	t.Helper()
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	orgID := newUUID(t, db)

	if _, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', $2, 'Quota Meter Org', 'active')`,
		orgID, "quota-meter-"+orgID[:8]); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     orgID,
		OrganizationSlug:   "quota-meter-" + orgID[:8],
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID:       newUUID(t, db),
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("tenant context: %v", err)
	}
	scope, err := coretenant.NewScope(tenantContext)
	if err != nil {
		t.Fatalf("scope: %v", err)
	}

	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		tx, err := db.Begin(cctx)
		if err != nil {
			t.Errorf("cleanup begin: %v", err)
			return
		}
		defer tx.Rollback(cctx)
		_, _ = tx.Exec(cctx, "SELECT set_config('app.organization_id', $1, true)", orgID)
		for _, table := range []string{"crm_activities", "crm_playbook_runs", "crm_leads", "crm_deals", "crm_companies", "crm_pipeline_stages", "crm_pipelines"} {
			_, _ = tx.Exec(cctx, "DELETE FROM "+table+" WHERE organization_id = $1", orgID)
		}
		_ = tx.Commit(cctx)
		_, _ = db.Exec(cctx, "DELETE FROM organizations WHERE id = $1", orgID)
	})

	reg := organizationservice.NewQuotaMeterRegistry()
	registerCRMQuotaMeters(reg, db)
	return db, scope, reg
}

func newUUID(t *testing.T, db *database.Pool) string {
	t.Helper()
	var id string
	if err := db.QueryRow(context.Background(), "SELECT gen_random_uuid()::text").Scan(&id); err != nil {
		t.Fatalf("gen uuid: %v", err)
	}
	return id
}

func meterUsed(t *testing.T, reg *organizationservice.QuotaMeterRegistry, scope coretenant.Scope, key string) int64 {
	t.Helper()
	meter, ok := reg.Get(key)
	if !ok {
		t.Fatalf("meter %s not registered", key)
	}
	used, err := meter.Used(context.Background(), scope.OrganizationID(), nil)
	if err != nil {
		t.Fatalf("Used(%s): %v", key, err)
	}
	return used
}

func createMeterLeads(t *testing.T, db *database.Pool, scope coretenant.Scope, n int) []string {
	t.Helper()
	repo := crmrepo.NewLeadRepository(db)
	var ids []string
	for i := 0; i < n; i++ {
		lead, err := repo.Create(context.Background(), scope, crmrepo.CreateLeadParams{
			ContactName: fmt.Sprintf("Lead %d", i), SkipPlaybook: true,
		})
		if err != nil {
			t.Fatalf("create lead: %v", err)
		}
		ids = append(ids, lead.ID)
	}
	return ids
}

func TestLeadMeterExcludesDeletedRecords(t *testing.T) {
	db, scope, reg := meterFixture(t)
	ids := createMeterLeads(t, db, scope, 3)
	if got := meterUsed(t, reg, scope, crmdomain.FeatureCRMMaxLeads); got != 3 {
		t.Fatalf("Used = %d, want 3", got)
	}
	if err := crmrepo.NewLeadRepository(db).Delete(context.Background(), scope, ids[0], ""); err != nil {
		t.Fatalf("delete lead: %v", err)
	}
	if got := meterUsed(t, reg, scope, crmdomain.FeatureCRMMaxLeads); got != 2 {
		t.Fatalf("Used after delete = %d, want 2", got)
	}
}

func TestLeadMeterCountsOnlyActiveStatuses(t *testing.T) {
	db, scope, reg := meterFixture(t)
	ids := createMeterLeads(t, db, scope, 3)
	if _, err := crmrepo.NewLeadRepository(db).Disqualify(context.Background(), scope, ids[0], crmrepo.DisqualifyLeadParams{
		Reason: crmdomain.DisqualifyNotFit,
	}); err != nil {
		t.Fatalf("disqualify: %v", err)
	}
	// "converted" is set directly: the full convert flow is covered elsewhere
	// and only the status filter matters to the meter.
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = tx.Exec(context.Background(), "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID())
	if _, err := tx.Exec(context.Background(),
		`UPDATE crm_leads SET status = 'converted', converted_at = now() WHERE id = $1 AND organization_id = $2`,
		ids[1], scope.OrganizationID()); err != nil {
		t.Fatalf("mark converted: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := meterUsed(t, reg, scope, crmdomain.FeatureCRMMaxLeads); got != 1 {
		t.Fatalf("Used = %d, want 1 (unqualified and converted excluded)", got)
	}
}

func TestCompanyMeterExcludesDeletedRecords(t *testing.T) {
	db, scope, reg := meterFixture(t)
	repo := crmrepo.NewCompanyRepository(db)
	var ids []string
	for i := 0; i < 3; i++ {
		c, err := repo.Create(context.Background(), scope, crmrepo.CreateCompanyParams{Name: fmt.Sprintf("PT %d", i)})
		if err != nil {
			t.Fatalf("create company: %v", err)
		}
		ids = append(ids, c.ID)
	}
	if err := repo.Delete(context.Background(), scope, ids[0], ""); err != nil {
		t.Fatalf("delete company: %v", err)
	}
	if got := meterUsed(t, reg, scope, crmdomain.FeatureCRMMaxCompanies); got != 2 {
		t.Fatalf("Used = %d, want 2", got)
	}
}

func TestDealMeterCountsOnlyOpenNonDeleted(t *testing.T) {
	db, scope, reg := meterFixture(t)
	ctx := context.Background()
	pipeline, err := crmrepo.NewPipelineRepository(db).Create(ctx, scope, crmrepo.CreatePipelineParams{
		Name: "Utama", IsDefault: true,
		Stages: []crmrepo.StageInput{
			{Name: "Baru", Position: 0, Probability: "10"},
			{Name: "Won", Position: 1, Probability: "100", IsWon: true},
			{Name: "Lost", Position: 2, Probability: "0", IsLost: true},
		},
	})
	if err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	deals := crmrepo.NewDealRepository(db)
	var ids []string
	for i := 0; i < 4; i++ {
		d, err := deals.Create(ctx, scope, crmrepo.CreateDealParams{
			PipelineID: pipeline.ID, StageID: pipeline.Stages[0].ID, Title: fmt.Sprintf("Deal %d", i),
		})
		if err != nil {
			t.Fatalf("create deal: %v", err)
		}
		ids = append(ids, d.ID)
	}
	if got := meterUsed(t, reg, scope, crmdomain.FeatureCRMMaxDeals); got != 4 {
		t.Fatalf("Used = %d, want 4", got)
	}
	if _, err := deals.CloseWon(ctx, scope, ids[0], ""); err != nil {
		t.Fatalf("close won: %v", err)
	}
	if _, err := deals.CloseLost(ctx, scope, ids[1], "harga", ""); err != nil {
		t.Fatalf("close lost: %v", err)
	}
	if err := deals.Delete(ctx, scope, ids[2], ""); err != nil {
		t.Fatalf("delete deal: %v", err)
	}
	if got := meterUsed(t, reg, scope, crmdomain.FeatureCRMMaxDeals); got != 1 {
		t.Fatalf("Used = %d, want 1 (won, lost, deleted excluded)", got)
	}
}

func TestPipelineMeterExcludesDeletedAndArchived(t *testing.T) {
	db, scope, reg := meterFixture(t)
	ctx := context.Background()
	repo := crmrepo.NewPipelineRepository(db)
	var ids []string
	for i := 0; i < 3; i++ {
		p, err := repo.Create(ctx, scope, crmrepo.CreatePipelineParams{
			Name: fmt.Sprintf("P%d", i), IsDefault: i == 0,
			Stages: []crmrepo.StageInput{{Name: "Baru", Position: 0, Probability: "10"}},
		})
		if err != nil {
			t.Fatalf("create pipeline: %v", err)
		}
		ids = append(ids, p.ID)
	}
	if _, err := repo.Archive(ctx, scope, ids[1], ""); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := repo.Delete(ctx, scope, ids[2], ""); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := meterUsed(t, reg, scope, crmdomain.FeatureCRMMaxPipelines); got != 1 {
		t.Fatalf("Used = %d, want 1", got)
	}
}

// Meters must be tenant-scoped: another organization's rows never count.
func TestLeadMeterIsTenantScoped(t *testing.T) {
	db, scope, reg := meterFixture(t)
	createMeterLeads(t, db, scope, 2)
	if got, err := mustMeter(t, reg, crmdomain.FeatureCRMMaxLeads).Used(context.Background(), newUUID(t, db), nil); err != nil || got != 0 {
		t.Fatalf("other org Used = %d, err = %v, want 0", got, err)
	}
}

func mustMeter(t *testing.T, reg *organizationservice.QuotaMeterRegistry, key string) organizationservice.QuotaMeter {
	t.Helper()
	m, ok := reg.Get(key)
	if !ok {
		t.Fatalf("meter %s not registered", key)
	}
	return m
}
