//go:build integration

package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestPageRepositoryLifecycleAndIsolationIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := repository.NewPageRepository(db)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	slugA := strings.ReplaceAll("page-"+testutil.UniqueCode("repoa"), ".", "-")
	slugB := strings.ReplaceAll("page-"+testutil.UniqueCode("repob"), ".", "-")

	// 1. Create a page for Tenant A
	pageA, err := repo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Page A",
		Title:      "Campaign A",
		Slug:       slugA,
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
		IsHomepage: false,
		CreatedBy:  "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create page for Tenant A: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, "DELETE FROM landing_pages WHERE id = $1", pageA.ID)
	})

	if pageA.Slug != slugA {
		t.Errorf("Expected slug %q, got %q", slugA, pageA.Slug)
	}

	// 2. Create a page for Tenant B
	pageB, err := repo.Create(ctx, tenants.B.Scope, repository.CreatePageParams{
		Name:       "Page B",
		Title:      "Campaign B",
		Slug:       slugB,
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
		IsHomepage: false,
		CreatedBy:  "22222222-2222-2222-2222-222222222222",
	})
	if err != nil {
		t.Fatalf("Create page for Tenant B: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, "DELETE FROM landing_pages WHERE id = $1", pageB.ID)
	})

	// 3. FindByID: Tenant A should find its own page
	foundA, err := repo.FindByID(ctx, tenants.A.Scope, pageA.ID)
	if err != nil {
		t.Errorf("Tenant A find own page: %v", err)
	}
	if foundA.ID != pageA.ID {
		t.Errorf("Expected page ID %q, got %q", pageA.ID, foundA.ID)
	}

	// 4. FindByID: Tenant B should NOT find Tenant A's page (Tenant Isolation)
	_, err = repo.FindByID(ctx, tenants.B.Scope, pageA.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("Tenant B FindByID page A: expected pgx.ErrNoRows, got %v", err)
	}

	// 5. Update: Tenant A updates its own page
	newName := "Updated Page A"
	updatedA, err := repo.Update(ctx, tenants.A.Scope, pageA.ID, repository.UpdatePageParams{
		Name:      &newName,
		UpdatedBy: "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Errorf("Tenant A Update own page: %v", err)
	}
	if updatedA.Name != newName {
		t.Errorf("Expected name %q, got %q", newName, updatedA.Name)
	}

	// 6. Update: Tenant B should NOT be able to update Tenant A's page
	dummyName := "Hacked Page Name"
	_, err = repo.Update(ctx, tenants.B.Scope, pageA.ID, repository.UpdatePageParams{
		Name:      &dummyName,
		UpdatedBy: "22222222-2222-2222-2222-222222222222",
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("Tenant B updating page A: expected pgx.ErrNoRows, got %v", err)
	}

	// 7. List: Tenant A listing pages should only see pageA, not pageB
	listA, totalA, err := repo.List(ctx, tenants.A.Scope, repository.PageListFilter{})
	if err != nil {
		t.Fatalf("Tenant A List: %v", err)
	}
	if totalA < 1 {
		t.Errorf("Expected total >= 1, got %d", totalA)
	}
	foundInListA := false
	for _, p := range listA {
		if p.ID == pageB.ID {
			t.Errorf("Tenant A List leaked Tenant B's page!")
		}
		if p.ID == pageA.ID {
			foundInListA = true
		}
	}
	if !foundInListA {
		t.Errorf("Tenant A List did not contain page A")
	}

	// 8. Delete: Tenant B should NOT be able to delete Tenant A's page
	err = repo.Delete(ctx, tenants.B.Scope, pageA.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("Tenant B deleting page A: expected pgx.ErrNoRows, got %v", err)
	}

	// 9. Delete: Tenant A deletes its own page (soft delete)
	err = repo.Delete(ctx, tenants.A.Scope, pageA.ID)
	if err != nil {
		t.Errorf("Tenant A Delete own page: %v", err)
	}

	// FindByID should now return pgx.ErrNoRows for Tenant A since it's soft-deleted
	_, err = repo.FindByID(ctx, tenants.A.Scope, pageA.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("FindByID soft-deleted page: expected pgx.ErrNoRows, got %v", err)
	}
}
