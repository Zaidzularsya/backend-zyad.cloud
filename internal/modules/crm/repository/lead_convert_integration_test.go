//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestConvertLeadCreatesAllRecordsAtomically(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)
	pipelineID, stages := createTestPipeline(t, db, tenants.A.Scope, "Utama", true)

	leads := repository.NewLeadRepository(db)
	lead, err := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Budi", CompanyName: "PT Maju"})
	if err != nil {
		t.Fatal(err)
	}

	res, err := leads.ConvertLead(ctx, tenants.A.Scope, lead.ID, repository.ConvertLeadTxParams{
		NewCompany: &repository.CreateCompanyParams{Name: "PT Maju"},
		Contact:    repository.CreateContactParams{FirstName: "Budi"},
		Deal: &repository.CreateDealParams{
			PipelineID: pipelineID, StageID: stages[1], Title: "Internet", Value: "15000000",
			Description: "50 Mbps", DecisionMaker: "Bu Rina",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Company == nil || res.Deal == nil {
		t.Fatalf("result = %+v", res)
	}
	if res.Contact.CompanyID == nil || *res.Contact.CompanyID != res.Company.ID {
		t.Fatalf("contact not linked to company: %+v", res.Contact)
	}
	if res.Deal.ContactID == nil || *res.Deal.ContactID != res.Contact.ID || res.Deal.StageID != stages[1] {
		t.Fatalf("deal links: %+v", res.Deal)
	}
	if res.Lead.ConvertedDealID == nil || *res.Lead.ConvertedDealID != res.Deal.ID {
		t.Fatalf("lead.converted_deal_id = %v", res.Lead.ConvertedDealID)
	}
	src, err := leads.FindSourceLeadByDealID(ctx, tenants.A.Scope, res.Deal.ID)
	if err != nil || src == nil || src.ID != lead.ID || src.ContactName != "Budi" {
		t.Fatalf("source lead = %+v err=%v", src, err)
	}
	// Tenant lain tidak melihat lead asal.
	if other, _ := leads.FindSourceLeadByDealID(ctx, tenants.B.Scope, res.Deal.ID); other != nil {
		t.Fatal("tenant B must not see the source lead")
	}
}

func TestConvertLeadRollsBackWhenDealFails(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)

	leads := repository.NewLeadRepository(db)
	lead, _ := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Citra"})

	_, err := leads.ConvertLead(ctx, tenants.A.Scope, lead.ID, repository.ConvertLeadTxParams{
		NewCompany: &repository.CreateCompanyParams{Name: "CV Gagal"},
		Contact:    repository.CreateContactParams{FirstName: "Citra"},
		// Pipeline tidak ada → FK gagal di tengah transaksi.
		Deal: &repository.CreateDealParams{PipelineID: "00000000-0000-0000-0000-000000000001", StageID: "00000000-0000-0000-0000-000000000002", Title: "X"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	contacts, _ := repository.NewContactRepository(db).Count(ctx, tenants.A.Scope, repository.ContactListFilter{})
	companies, total, _ := repository.NewCompanyRepository(db).List(ctx, tenants.A.Scope, repository.CompanyListFilter{Limit: 10})
	if contacts != 0 || total != 0 || len(companies) != 0 {
		t.Fatalf("leftovers: contacts=%d companies=%d", contacts, total)
	}
	again, _ := leads.FindByID(ctx, tenants.A.Scope, lead.ID)
	if again.Status == "converted" {
		t.Fatal("lead must stay unconverted")
	}
}

func TestConvertLeadTwiceFailsWithoutDuplicates(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)

	leads := repository.NewLeadRepository(db)
	lead, _ := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Dedi"})
	params := repository.ConvertLeadTxParams{Contact: repository.CreateContactParams{FirstName: "Dedi"}}
	if _, err := leads.ConvertLead(ctx, tenants.A.Scope, lead.ID, params); err != nil {
		t.Fatal(err)
	}
	_, err := leads.ConvertLead(ctx, tenants.A.Scope, lead.ID, params)
	if !errors.Is(err, repository.ErrLeadAlreadyConverted) {
		t.Fatalf("second convert err = %v, want ErrLeadAlreadyConverted", err)
	}
	if n, _ := repository.NewContactRepository(db).Count(ctx, tenants.A.Scope, repository.ContactListFilter{}); n != 1 {
		t.Fatalf("contacts = %d, want 1", n)
	}
}

func TestConvertLeadWithExistingCompany(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)

	company, _ := repository.NewCompanyRepository(db).Create(ctx, tenants.A.Scope, repository.CreateCompanyParams{Name: "PT Maju Jaya"})
	leads := repository.NewLeadRepository(db)
	lead, _ := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Eka"})
	res, err := leads.ConvertLead(ctx, tenants.A.Scope, lead.ID, repository.ConvertLeadTxParams{
		ExistingCompanyID: company.ID,
		Contact:           repository.CreateContactParams{FirstName: "Eka"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Company == nil || res.Company.ID != company.ID || res.Deal != nil {
		t.Fatalf("result = %+v", res)
	}
	if res.Lead.ConvertedCompanyID == nil || *res.Lead.ConvertedCompanyID != company.ID {
		t.Fatal("lead must reference the existing company")
	}
}

func TestAttachDealToConvertedLead(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	disablePlaybook(t, db, tenants.A.Scope)
	pipelineID, stages := createTestPipeline(t, db, tenants.A.Scope, "Utama", true)

	leads := repository.NewLeadRepository(db)
	open, _ := leads.Create(ctx, tenants.A.Scope, repository.CreateLeadParams{ContactName: "Fajar"})
	dealParams := repository.CreateDealParams{PipelineID: pipelineID, StageID: stages[0], Title: "Deal Fajar"}

	if _, _, err := leads.AttachDeal(ctx, tenants.A.Scope, open.ID, dealParams); !errors.Is(err, repository.ErrLeadNotConverted) {
		t.Fatalf("unconverted lead err = %v", err)
	}
	conv, _ := leads.ConvertLead(ctx, tenants.A.Scope, open.ID, repository.ConvertLeadTxParams{Contact: repository.CreateContactParams{FirstName: "Fajar"}})

	lead, deal, err := leads.AttachDeal(ctx, tenants.A.Scope, open.ID, dealParams)
	if err != nil {
		t.Fatal(err)
	}
	if deal.ContactID == nil || *deal.ContactID != conv.Contact.ID || lead.ConvertedDealID == nil || *lead.ConvertedDealID != deal.ID {
		t.Fatalf("lead=%+v deal=%+v", lead, deal)
	}
	if _, _, err := leads.AttachDeal(ctx, tenants.A.Scope, open.ID, dealParams); !errors.Is(err, repository.ErrLeadDealExists) {
		t.Fatalf("second attach err = %v", err)
	}
}

func TestFindCandidatesByNameIsTenantScopedAndEscapesWildcards(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)

	companies := repository.NewCompanyRepository(db)
	for _, n := range []string{"PT Maju Jaya", "CV Maju 100%"} {
		if _, err := companies.Create(ctx, tenants.A.Scope, repository.CreateCompanyParams{Name: n}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := companies.Create(ctx, tenants.B.Scope, repository.CreateCompanyParams{Name: "PT Maju Lain"}); err != nil {
		t.Fatal(err)
	}
	got, err := companies.FindCandidatesByName(ctx, tenants.A.Scope, "maju", 25)
	if err != nil || len(got) != 2 {
		t.Fatalf("maju → %d err=%v, want 2 (tenant A only)", len(got), err)
	}
	if got, _ := companies.FindCandidatesByName(ctx, tenants.A.Scope, "%", 25); len(got) != 1 {
		t.Fatalf("literal %% must match only the name containing it, got %d", len(got))
	}
}
