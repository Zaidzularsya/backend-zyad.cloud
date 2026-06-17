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

func TestSectionRepositoryLifecycleAndIsolationIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	pageRepo := repository.NewPageRepository(db)
	sectionRepo := repository.NewSectionRepository(db)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	slugA := strings.ReplaceAll("page-"+testutil.UniqueCode("secrepoa"), ".", "-")
	slugB := strings.ReplaceAll("page-"+testutil.UniqueCode("secrepob"), ".", "-")

	// 1. Create a page for Tenant A
	pageA, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
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

	// 2. Create a page for Tenant B
	pageB, err := pageRepo.Create(ctx, tenants.B.Scope, repository.CreatePageParams{
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

	// 3. Create a section in Page A (Tenant A)
	sectionA, err := sectionRepo.Create(ctx, tenants.A.Scope, repository.CreateSectionParams{
		LandingPageID: pageA.ID,
		Key:           "hero-1",
		Type:          domain.SectionTypeHero,
		Name:          "Hero Section A",
		SortOrder:     0,
		IsEnabled:     true,
		Content:       map[string]any{"headline": "Welcome A"},
		Style:         map[string]any{"bg": "white"},
	})
	if err != nil {
		t.Fatalf("Create section in page A: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, "DELETE FROM landing_page_sections WHERE id = $1", sectionA.ID)
	})

	// 4. Create a section in Page B (Tenant B)
	sectionB, err := sectionRepo.Create(ctx, tenants.B.Scope, repository.CreateSectionParams{
		LandingPageID: pageB.ID,
		Key:           "hero-2",
		Type:          domain.SectionTypeHero,
		Name:          "Hero Section B",
		SortOrder:     0,
		IsEnabled:     true,
		Content:       map[string]any{"headline": "Welcome B"},
		Style:         map[string]any{"bg": "black"},
	})
	if err != nil {
		t.Fatalf("Create section in page B: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, "DELETE FROM landing_page_sections WHERE id = $1", sectionB.ID)
	})

	// 5. FindByID: Tenant A should find its own section
	foundSecA, err := sectionRepo.FindByID(ctx, tenants.A.Scope, sectionA.ID)
	if err != nil {
		t.Errorf("Tenant A FindByID own section: %v", err)
	}
	if foundSecA.ID != sectionA.ID {
		t.Errorf("Expected section ID %q, got %q", sectionA.ID, foundSecA.ID)
	}

	// 6. FindByID: Tenant B should NOT find Tenant A's section
	_, err = sectionRepo.FindByID(ctx, tenants.B.Scope, sectionA.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("Tenant B FindByID section A: expected pgx.ErrNoRows, got %v", err)
	}

	// 7. ListByPage: Tenant A should list sections in pageA
	listA, err := sectionRepo.ListByPage(ctx, tenants.A.Scope, pageA.ID)
	if err != nil {
		t.Fatalf("Tenant A ListByPage page A: %v", err)
	}
	if len(listA) != 1 || listA[0].ID != sectionA.ID {
		t.Errorf("Expected [sectionA], got %v", listA)
	}

	// 8. ListByPage: Tenant B should NOT be able to list sections in pageA
	listB, err := sectionRepo.ListByPage(ctx, tenants.B.Scope, pageA.ID)
	if err != nil {
		t.Fatalf("Tenant B ListByPage page A: %v", err)
	}
	// Under RLS or tenant organization ID predicate, this query returns empty list since pageA belongs to tenant A
	if len(listB) != 0 {
		t.Errorf("Tenant B ListByPage page A: expected 0 items, got %d", len(listB))
	}

	// 9. Update: Tenant A updates its section
	updatedName := "Hero Section A (New)"
	updatedSecA, err := sectionRepo.Update(ctx, tenants.A.Scope, sectionA.ID, repository.UpdateSectionParams{
		Name: &updatedName,
	})
	if err != nil {
		t.Errorf("Tenant A Update own section: %v", err)
	}
	if updatedSecA.Name != updatedName {
		t.Errorf("Expected name %q, got %q", updatedName, updatedSecA.Name)
	}

	// 10. Update: Tenant B should NOT be able to update Tenant A's section
	hackedName := "Hacked"
	_, err = sectionRepo.Update(ctx, tenants.B.Scope, sectionA.ID, repository.UpdateSectionParams{
		Name: &hackedName,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("Tenant B updating section A: expected pgx.ErrNoRows, got %v", err)
	}

	// 11. Reorder: Tenant A reorders its section
	err = sectionRepo.Reorder(ctx, tenants.A.Scope, pageA.ID, []repository.SectionReorderParam{
		{ID: sectionA.ID, SortOrder: 5},
	})
	if err != nil {
		t.Errorf("Tenant A Reorder section: %v", err)
	}

	// Verify SortOrder changed
	foundSecA, err = sectionRepo.FindByID(ctx, tenants.A.Scope, sectionA.ID)
	if err != nil {
		t.Fatalf("FindByID after reorder: %v", err)
	}
	if foundSecA.SortOrder != 5 {
		t.Errorf("Expected SortOrder 5, got %d", foundSecA.SortOrder)
	}

	// 12. Delete: Tenant B should NOT be able to delete Tenant A's section
	err = sectionRepo.Delete(ctx, tenants.B.Scope, sectionA.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("Tenant B deleting section A: expected pgx.ErrNoRows, got %v", err)
	}

	// 13. Delete: Tenant A deletes its own section (soft delete)
	err = sectionRepo.Delete(ctx, tenants.A.Scope, sectionA.ID)
	if err != nil {
		t.Errorf("Tenant A Delete own section: %v", err)
	}

	// FindByID should now return pgx.ErrNoRows for Tenant A since it's soft-deleted
	_, err = sectionRepo.FindByID(ctx, tenants.A.Scope, sectionA.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("FindByID soft-deleted section: expected pgx.ErrNoRows, got %v", err)
	}
}
