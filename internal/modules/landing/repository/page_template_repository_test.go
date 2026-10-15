//go:build integration

package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func ptr[T any](v T) *T { return &v }

func newTemplateParams(slug string, status domain.PageTemplateStatus, platformOnly bool) repository.CreatePageTemplateParams {
	return repository.CreatePageTemplateParams{
		Slug: slug, Name: "Tpl " + slug, Description: "desc", Category: "dasar",
		SortOrder: 100, Status: status, PlatformOnly: platformOnly,
		HTML: "<section>a</section>", CSS: ".a{color:red}",
		Source: domain.PageTemplateSourceAdmin,
	}
}

func TestPageTemplateRepository_CreateFindList(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	repo := repository.NewPageTemplateRepository(db)
	pfx := strings.ReplaceAll(testutil.UniqueCode("ptl"), ".", "-")
	t.Cleanup(func() { db.Exec(ctx, "DELETE FROM landing_page_templates WHERE slug LIKE $1", pfx+"%") })

	a, err := repo.Create(ctx, newTemplateParams(pfx+"-a", domain.PageTemplateStatusPublished, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, newTemplateParams(pfx+"-b", domain.PageTemplateStatusDraft, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, newTemplateParams(pfx+"-c", domain.PageTemplateStatusPublished, true)); err != nil {
		t.Fatal(err)
	}

	count := func(f repository.PageTemplateListFilter) int {
		items, err := repo.List(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, it := range items {
			if strings.HasPrefix(it.Slug, pfx) {
				n++
				if it.HTML != "" || it.CSS != "" {
					t.Errorf("List must not return html/css")
				}
			}
		}
		return n
	}
	pub := []domain.PageTemplateStatus{domain.PageTemplateStatusPublished}
	if n := count(repository.PageTemplateListFilter{Statuses: pub}); n != 1 {
		t.Errorf("published without platform_only: got %d want 1", n)
	}
	if n := count(repository.PageTemplateListFilter{Statuses: pub, IncludePlatformOnly: true}); n != 2 {
		t.Errorf("published with platform_only: got %d want 2", n)
	}
	if n := count(repository.PageTemplateListFilter{IncludePlatformOnly: true}); n != 3 {
		t.Errorf("all: got %d want 3", n)
	}

	got, err := repo.FindByID(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.HTML != "<section>a</section>" || got.CSS != ".a{color:red}" {
		t.Errorf("FindByID must return html/css, got %q %q", got.HTML, got.CSS)
	}
	bySlug, err := repo.FindBySlug(ctx, pfx+"-a")
	if err != nil || bySlug.ID != a.ID {
		t.Errorf("FindBySlug: %v %v", bySlug.ID, err)
	}
}

func TestPageTemplateRepository_SlugTaken(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	repo := repository.NewPageTemplateRepository(db)
	slug := strings.ReplaceAll(testutil.UniqueCode("ptd"), ".", "-")
	t.Cleanup(func() { db.Exec(ctx, "DELETE FROM landing_page_templates WHERE slug = $1", slug) })

	if _, err := repo.Create(ctx, newTemplateParams(slug, domain.PageTemplateStatusDraft, false)); err != nil {
		t.Fatal(err)
	}
	_, err := repo.Create(ctx, newTemplateParams(slug, domain.PageTemplateStatusDraft, false))
	if !errors.Is(err, repository.ErrPageTemplateSlugTaken) {
		t.Fatalf("want ErrPageTemplateSlugTaken, got %v", err)
	}
}

func TestPageTemplateRepository_UpdatePartial(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	repo := repository.NewPageTemplateRepository(db)
	slug := strings.ReplaceAll(testutil.UniqueCode("ptu"), ".", "-")
	t.Cleanup(func() { db.Exec(ctx, "DELETE FROM landing_page_templates WHERE slug = $1", slug) })

	created, err := repo.Create(ctx, newTemplateParams(slug, domain.PageTemplateStatusDraft, false))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	archived := domain.PageTemplateStatusArchived
	upd, err := repo.Update(ctx, created.ID, repository.UpdatePageTemplateParams{Status: &archived})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Status != archived || upd.Name != created.Name {
		t.Errorf("status-only update changed other fields: %+v", upd)
	}

	upd2, err := repo.Update(ctx, created.ID, repository.UpdatePageTemplateParams{HTML: ptr("<p>baru</p>"), SeedModified: ptr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if upd2.HTML != "<p>baru</p>" || !upd2.SeedModified {
		t.Errorf("html/seed_modified not updated: %+v", upd2)
	}
	if !upd2.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("updated_at must advance: %v -> %v", created.UpdatedAt, upd2.UpdatedAt)
	}
}

func TestPageTemplateRepository_FindByID_BadUUID(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	repo := repository.NewPageTemplateRepository(db)
	if _, err := repo.FindByID(context.Background(), "bukan-uuid"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("want pgx.ErrNoRows, got %v", err)
	}
}

func TestPageTemplateRepository_SourcePageSetNullOnDelete(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	_, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'organization-ptr', 'Organization PTR', 'active')
		ON CONFLICT DO NOTHING`, tenants.A.OrganizationID)
	if err != nil {
		t.Fatal(err)
	}
	pageRepo := repository.NewPageRepository(db)
	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name: "Src", Title: "Src", Slug: strings.ReplaceAll("src-"+testutil.UniqueCode("ptr"), ".", "-"),
		Type: domain.PageTypeCampaign, Status: domain.PageStatusDraft, Visibility: domain.PageVisibilityPublic,
		Locale: "id-ID", Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatal(err)
	}

	repo := repository.NewPageTemplateRepository(db)
	slug := strings.ReplaceAll(testutil.UniqueCode("pts"), ".", "-")
	t.Cleanup(func() { db.Exec(ctx, "DELETE FROM landing_page_templates WHERE slug = $1", slug) })
	p := newTemplateParams(slug, domain.PageTemplateStatusDraft, false)
	p.SourcePageID = &page.ID
	tpl, err := repo.Create(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if tpl.SourcePageID == nil || *tpl.SourcePageID != page.ID {
		t.Fatalf("source_page_id not stored: %v", tpl.SourcePageID)
	}
	if _, err := db.Exec(ctx, "DELETE FROM landing_pages WHERE id = $1", page.ID); err != nil {
		t.Fatal(err)
	}
	after, err := repo.FindByID(ctx, tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.SourcePageID != nil {
		t.Errorf("source_page_id should be NULL after page delete, got %v", *after.SourcePageID)
	}
}
