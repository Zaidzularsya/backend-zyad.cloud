//go:build integration

package repository_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestContactFindDuplicate_NormalizedAndScoped(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	contacts := repository.NewContactRepository(db)

	existing, err := contacts.Create(ctx, tenants.A.Scope, repository.CreateContactParams{
		FirstName: "Budi", Email: " Budi@Example.com ", Phone: "0812-3456-7890",
	})
	if err != nil {
		t.Fatal(err)
	}

	got, found, err := contacts.FindDuplicate(ctx, tenants.A.Scope, "budi@example.com", "", "")
	if err != nil || !found || got.ID != existing.ID {
		t.Fatalf("email lookup = %v %v %v", got.ID, found, err)
	}
	got, found, err = contacts.FindDuplicate(ctx, tenants.A.Scope, "", "6281234567890", "")
	if err != nil || !found || got.ID != existing.ID {
		t.Fatalf("phone lookup = %v %v %v", got.ID, found, err)
	}
	if _, found, _ := contacts.FindDuplicate(ctx, tenants.A.Scope, "budi@example.com", "", existing.ID); found {
		t.Fatal("excludeID must skip the contact itself")
	}
	if _, found, _ := contacts.FindDuplicate(ctx, tenants.B.Scope, "budi@example.com", "6281234567890", ""); found {
		t.Fatal("tenant B must not see tenant A contacts")
	}
	if err := contacts.Delete(ctx, tenants.A.Scope, existing.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := contacts.FindDuplicate(ctx, tenants.A.Scope, "budi@example.com", "", ""); found {
		t.Fatal("deleted contacts are not duplicates")
	}
}

func TestDealSourceStoredAndFiltered(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)
	pipelineID, stages := createTestPipeline(t, db, tenants.A.Scope, "Utama", true)
	deals := repository.NewDealRepository(db)

	imported, err := deals.Create(ctx, tenants.A.Scope, repository.CreateDealParams{
		PipelineID: pipelineID, StageID: stages[0], Title: "PPPoE", Source: "Impor PPPoE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deals.Create(ctx, tenants.A.Scope, repository.CreateDealParams{PipelineID: pipelineID, StageID: stages[0], Title: "Manual"}); err != nil {
		t.Fatal(err)
	}
	if imported.Source != "Impor PPPoE" {
		t.Fatalf("source = %q", imported.Source)
	}
	list, total, err := deals.List(ctx, tenants.A.Scope, repository.DealListFilter{Source: "Impor PPPoE", Limit: 10})
	if err != nil || total != 1 || len(list) != 1 || list[0].ID != imported.ID {
		t.Fatalf("filtered = %d %v err=%v", total, list, err)
	}
}
