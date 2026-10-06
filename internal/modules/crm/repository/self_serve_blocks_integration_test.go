//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestCreateLeadSkipPlaybook(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	leads := repository.NewLeadRepository(db)

	runs := func(leadID string) int {
		var n int
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		_, _ = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", tenants.A.OrganizationID)
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM crm_playbook_runs WHERE entity_type = 'lead' AND entity_id = $1`, leadID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	skipped, err := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Rina", Source: "landing_page", SkipPlaybook: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := runs(skipped.ID); n != 0 {
		t.Fatalf("SkipPlaybook: runs = %d, want 0", n)
	}
	// regresi: tanpa flag, playbook default tetap membuat run
	normal, err := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Sari"})
	if err != nil {
		t.Fatal(err)
	}
	if n := runs(normal.ID); n != 1 {
		t.Fatalf("default: runs = %d, want 1", n)
	}
}

func TestConvertLeadExistingContact(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)
	pipelineID, stages := createTestPipeline(t, db, tenants.A.Scope, "Utama", true)

	contacts := repository.NewContactRepository(db)
	company, _ := repository.NewCompanyRepository(db).Create(ctx, tenants.A.Scope, repository.CreateCompanyParams{Name: "Studio Rina"})
	existing, err := contacts.Create(ctx, tenants.A.Scope, repository.CreateContactParams{FirstName: "Rina", CompanyID: company.ID})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := contacts.Create(ctx, tenants.B.Scope, repository.CreateContactParams{FirstName: "Orang Lain"})
	if err != nil {
		t.Fatal(err)
	}

	leads := repository.NewLeadRepository(db)
	lead, _ := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Rina"})
	before, _ := contacts.Count(ctx, tenants.A.Scope, repository.ContactListFilter{})

	// contact milik org lain → tidak ditemukan, tidak ada yang tertulis
	if _, err := leads.ConvertLead(ctx, tenants.A.Scope, lead.ID, repository.ConvertLeadTxParams{ExistingContactID: foreign.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign contact err = %v, want ErrNoRows", err)
	}

	res, err := leads.ConvertLead(ctx, tenants.A.Scope, lead.ID, repository.ConvertLeadTxParams{
		ExistingCompanyID: company.ID,
		ExistingContactID: existing.ID,
		Deal:              &repository.CreateDealParams{PipelineID: pipelineID, StageID: stages[0], Title: "Deal", Value: "100"},
	})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := contacts.Count(ctx, tenants.A.Scope, repository.ContactListFilter{})
	if after != before {
		t.Fatalf("contacts %d -> %d, want unchanged", before, after)
	}
	if res.Contact.ID != existing.ID || res.Deal == nil || res.Deal.ContactID == nil || *res.Deal.ContactID != existing.ID {
		t.Fatalf("result = %+v", res)
	}
	if res.Lead.ConvertedContactID == nil || *res.Lead.ConvertedContactID != existing.ID {
		t.Fatalf("lead.converted_contact_id = %v", res.Lead.ConvertedContactID)
	}
}

func TestCompanyTenantLink(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	companies := repository.NewCompanyRepository(db)
	workspace := tenants.B.OrganizationID

	a, _ := companies.Create(ctx, tenants.A.Scope, repository.CreateCompanyParams{Name: "Studio Rina"})
	b, _ := companies.Create(ctx, tenants.A.Scope, repository.CreateCompanyParams{Name: "Studio Rina Lain"})

	if _, err := companies.FindByTenantOrganization(ctx, tenants.A.Scope, workspace); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("before link err = %v", err)
	}
	linked, err := companies.SetTenantOrganization(ctx, tenants.A.Scope, a.ID, workspace, "")
	if err != nil || linked.TenantOrganizationID == nil || *linked.TenantOrganizationID != workspace {
		t.Fatalf("set: %+v err=%v", linked, err)
	}
	found, err := companies.FindByTenantOrganization(ctx, tenants.A.Scope, workspace)
	if err != nil || found.ID != a.ID {
		t.Fatalf("find: %+v err=%v", found, err)
	}
	if _, err := companies.SetTenantOrganization(ctx, tenants.A.Scope, b.ID, workspace, ""); !errors.Is(err, repository.ErrWorkspaceAlreadyLinked) {
		t.Fatalf("second link err = %v, want ErrWorkspaceAlreadyLinked", err)
	}
	// org lain tidak menemukan
	if _, err := companies.FindByTenantOrganization(ctx, tenants.B.Scope, workspace); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other org err = %v", err)
	}
}

func TestFindPipelineBySystemKey(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	pipelines := repository.NewPipelineRepository(db)

	if _, err := pipelines.FindBySystemKey(ctx, tenants.A.Scope, "self_serve"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing err = %v", err)
	}
	id, _ := createTestPipeline(t, db, tenants.A.Scope, "Self-Serve", false)
	if code := execInOrg(t, db, tenants.A.OrganizationID, `UPDATE crm_pipelines SET system_key = 'self_serve' WHERE id = $1`, id); code != "" {
		t.Fatal(code)
	}
	p, err := pipelines.FindBySystemKey(ctx, tenants.A.Scope, "self_serve")
	if err != nil || p.ID != id || len(p.Stages) != 4 || p.Stages[0].Position != 0 || p.Stages[3].Position != 3 {
		t.Fatalf("pipeline = %+v err=%v", p, err)
	}
	if _, err := pipelines.FindBySystemKey(ctx, tenants.B.Scope, "self_serve"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("other org err = %v", err)
	}
}

func TestListOpenDealsByCompanyAndPipeline(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	pipelineID, stages := createTestPipeline(t, db, tenants.A.Scope, "Utama", true)
	otherPipeline, otherStages := createTestPipeline(t, db, tenants.A.Scope, "Lain", false)
	company, _ := repository.NewCompanyRepository(db).Create(ctx, tenants.A.Scope, repository.CreateCompanyParams{Name: "Studio Rina"})
	deals := repository.NewDealRepository(db)

	mk := func(pipeline, stage, title string) domain.Deal {
		d, err := deals.Create(ctx, tenants.A.Scope, repository.CreateDealParams{PipelineID: pipeline, StageID: stage, CompanyID: company.ID, Title: title, Value: "1"})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	open := mk(pipelineID, stages[0], "open")
	lost := mk(pipelineID, stages[0], "lost")
	deleted := mk(pipelineID, stages[0], "deleted")
	mk(otherPipeline, otherStages[0], "other pipeline")
	if _, err := deals.CloseLost(ctx, tenants.A.Scope, lost.ID, "x", ""); err != nil {
		t.Fatal(err)
	}
	if err := deals.Delete(ctx, tenants.A.Scope, deleted.ID, ""); err != nil {
		t.Fatal(err)
	}

	got, err := deals.ListOpenByCompanyAndPipeline(ctx, tenants.A.Scope, company.ID, pipelineID)
	if err != nil || len(got) != 1 || got[0].ID != open.ID {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if other, _ := deals.ListOpenByCompanyAndPipeline(ctx, tenants.B.Scope, company.ID, pipelineID); len(other) != 0 {
		t.Fatal("tenant B must not see deals")
	}
}

func TestConvertLeadNewCompanyLinkedToWorkspace(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)
	leads := repository.NewLeadRepository(db)
	lead, _ := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Rina"})

	res, err := leads.ConvertLead(ctx, tenants.A.Scope, lead.ID, repository.ConvertLeadTxParams{
		NewCompany: &repository.CreateCompanyParams{Name: "Studio Rina", TenantOrganizationID: tenants.B.OrganizationID},
		Contact:    repository.CreateContactParams{FirstName: "Rina"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Company == nil || res.Company.TenantOrganizationID == nil || *res.Company.TenantOrganizationID != tenants.B.OrganizationID {
		t.Fatalf("company = %+v", res.Company)
	}
	found, err := repository.NewCompanyRepository(db).FindByTenantOrganization(ctx, tenants.A.Scope, tenants.B.OrganizationID)
	if err != nil || found.ID != res.Company.ID {
		t.Fatalf("find: %+v err=%v", found, err)
	}
}
