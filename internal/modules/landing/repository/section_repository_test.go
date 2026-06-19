//go:build integration

package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestSectionRepositoryLifecycleAndIsolationIntegration(t *testing.T) {
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
	sectionRepo := repository.NewSectionRepository(db)

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

type sectionIsolationAdapter struct {
	repo       repository.SectionRepository
	pageRepo   repository.PageRepository
	ids        map[string]string
	scopePages map[string]string
}

func (a *sectionIsolationAdapter) ensurePage(ctx context.Context, scope coretenant.Scope) (string, error) {
	pageID, ok := a.scopePages[scope.OrganizationID()]
	if ok {
		return pageID, nil
	}
	page, err := a.pageRepo.Create(ctx, scope, repository.CreatePageParams{
		Name:       "Test Page " + scope.OrganizationID(),
		Title:      "Test",
		Slug:       strings.ReplaceAll("testpage-"+testutil.UniqueCode("sec")+scope.OrganizationID()[:8], ".", "-"),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		Locale:     "id-ID",
		Timezone:   "Asia/Jakarta",
	})
	if err != nil {
		return "", err
	}
	a.scopePages[scope.OrganizationID()] = page.ID
	return page.ID, nil
}

func (a *sectionIsolationAdapter) Create(ctx context.Context, tctx coretenant.Context, key string, value string) error {
	scope, err := coretenant.NewScope(tctx)
	if err != nil {
		return err
	}
	pageID, err := a.ensurePage(ctx, scope)
	if err != nil {
		return err
	}
	section, err := a.repo.Create(ctx, scope, repository.CreateSectionParams{
		LandingPageID: pageID,
		Key:           strings.ReplaceAll(key, "_", "-"),
		Type:          domain.SectionTypeHero,
		Name:          value,
		SortOrder:     0,
		IsEnabled:     true,
		Content:       make(map[string]any),
		Style:         make(map[string]any),
	})
	if err != nil {
		return err
	}
	a.ids[key] = section.ID
	return nil
}

func (a *sectionIsolationAdapter) Read(ctx context.Context, tctx coretenant.Context, key string) (string, bool, error) {
	scope, err := coretenant.NewScope(tctx)
	if err != nil {
		return "", false, err
	}
	id, ok := a.ids[key]
	if !ok {
		return "", false, nil
	}
	section, err := a.repo.FindByID(ctx, scope, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return "", false, nil
		}
		return "", false, err
	}
	return section.Name, true, nil
}

func (a *sectionIsolationAdapter) List(ctx context.Context, tctx coretenant.Context) ([]string, error) {
	scope, err := coretenant.NewScope(tctx)
	if err != nil {
		return nil, err
	}
	pageID, err := a.ensurePage(ctx, scope)
	if err != nil {
		return nil, err
	}
	sections, err := a.repo.ListByPage(ctx, scope, pageID)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, s := range sections {
		for k, id := range a.ids {
			if id == s.ID {
				keys = append(keys, k)
				break
			}
		}
	}
	return keys, nil
}

func (a *sectionIsolationAdapter) Update(ctx context.Context, tctx coretenant.Context, key string, value string) (bool, error) {
	scope, err := coretenant.NewScope(tctx)
	if err != nil {
		return false, err
	}
	id, ok := a.ids[key]
	if !ok {
		return false, nil
	}
	_, err = a.repo.Update(ctx, scope, id, repository.UpdateSectionParams{
		Name: &value,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (a *sectionIsolationAdapter) Delete(ctx context.Context, tctx coretenant.Context, key string) (bool, error) {
	scope, err := coretenant.NewScope(tctx)
	if err != nil {
		return false, err
	}
	id, ok := a.ids[key]
	if !ok {
		return false, nil
	}
	err = a.repo.Delete(ctx, scope, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (a *sectionIsolationAdapter) BulkUpdate(ctx context.Context, tctx coretenant.Context, keys []string, value string) (int64, error) {
	var count int64
	for _, k := range keys {
		updated, err := a.Update(ctx, tctx, k, value)
		if err != nil {
			return count, err
		}
		if updated {
			count++
		}
	}
	return count, nil
}

func (a *sectionIsolationAdapter) Export(ctx context.Context, tctx coretenant.Context) ([]string, error) {
	return a.List(ctx, tctx)
}

func TestSectionTenantIsolationSuite(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := repository.NewSectionRepository(db)
	pageRepo := repository.NewPageRepository(db)
	adapter := &sectionIsolationAdapter{
		repo:       repo,
		pageRepo:   pageRepo,
		ids:        make(map[string]string),
		scopePages: make(map[string]string),
	}
	testutil.RunTenantIsolationSuite(t, adapter)
}
