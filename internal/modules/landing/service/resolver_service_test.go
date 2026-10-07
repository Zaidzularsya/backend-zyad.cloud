//go:build integration

package service_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestResolverServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	userID := "22222222-2222-2222-2222-222222222222"
	_, err := db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES ($1, 'Mock User B', 'mock_b@example.com', 'active')
		ON CONFLICT DO NOTHING
	`, userID)
	if err != nil {
		t.Fatalf("failed to insert mock user: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'organization-b', 'Organization B', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID)
	if err != nil {
		t.Fatalf("failed to insert mock org: %v", err)
	}

	pageRepo := repository.NewPageRepository(db)
	sectionRepo := repository.NewSectionRepository(db)
	formRepo := repository.NewFormRepository(db)
	brandingRepo := repository.NewBrandingRepository(db)
	versionRepo := repository.NewVersionRepository(db)
	documentRepo := repository.NewDocumentRepository(db)
	resolverRepo := repository.NewResolverRepository(db)
	reusableRepo := repository.NewReusableRepository(db)

	publishService := service.NewPublishService(db, pageRepo, sectionRepo, formRepo, brandingRepo, versionRepo, documentRepo, "test-secret")
	resolverService := service.NewResolverService(db, resolverRepo, versionRepo, pageRepo, sectionRepo, formRepo, brandingRepo, reusableRepo, documentRepo, publishService)

	slug := strings.ReplaceAll(testutil.UniqueCode("resolve-page-"), ".", "-")

	// Create Page
	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Resolve Test Page",
		Title:      "Resolve Title",
		Slug:       slug,
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		CreatedBy:  userID,
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	t.Run("ResolveBySlug - Draft without Token", func(t *testing.T) {
		_, err := resolverService.ResolveBySlug(ctx, tenants.A.Scope, slug, "")
		if err != service.ErrPageNotPublished {
			t.Errorf("Expected ErrPageNotPublished, got %v", err)
		}
	})

	t.Run("ResolveBySlug - Draft with Valid Token", func(t *testing.T) {
		token, _ := publishService.GeneratePreviewToken(ctx, tenants.A.Scope, page.ID, 60)

		resolved, err := resolverService.ResolveBySlug(ctx, tenants.A.Scope, slug, token)
		if err != nil {
			t.Fatalf("ResolveBySlug: %v", err)
		}
		if resolved.Page.ID != page.ID {
			t.Errorf("Expected page ID %s, got %s", page.ID, resolved.Page.ID)
		}
		if !resolved.IsDraft {
			t.Errorf("Expected IsDraft to be true")
		}
	})

	t.Run("ResolveBySlug - Published", func(t *testing.T) {
		// Publish the page first
		_, err := publishService.Publish(ctx, tenants.A.Scope, page.ID, "Initial Release", userID)
		if err != nil {
			t.Fatalf("Publish: %v", err)
		}

		resolved, err := resolverService.ResolveBySlug(ctx, tenants.A.Scope, slug, "")
		if err != nil {
			t.Fatalf("ResolveBySlug: %v", err)
		}
		if resolved.Page.ID != page.ID {
			t.Errorf("Expected page ID %s, got %s", page.ID, resolved.Page.ID)
		}
		if resolved.IsDraft {
			t.Errorf("Expected IsDraft to be false")
		}
		if resolved.Snapshot == nil {
			t.Errorf("Expected snapshot to be populated")
		}
	})

	t.Run("ResolveBySlug - Not Found", func(t *testing.T) {
		_, err := resolverService.ResolveBySlug(ctx, tenants.A.Scope, "non-existent-slug", "")
		if err != service.ErrPageNotFound {
			t.Errorf("Expected ErrPageNotFound, got %v", err)
		}
	})

	// FTR-BE-002: resolve payload must always carry Branding/Menus/Page.Settings
	// so the renderer can derive footer content, regardless of which sections
	// the page has or whether the resolve hits the published snapshot path.
	t.Run("ResolveBySlug - Published page always carries Branding/Menus/Settings for footer", func(t *testing.T) {
		footerSlug := strings.ReplaceAll(testutil.UniqueCode("resolve-footer-page-"), ".", "-")

		footerPage, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
			Name:       "Resolve Footer Test Page",
			Title:      "Resolve Footer Title",
			Slug:       footerSlug,
			Type:       domain.PageTypeCampaign,
			Status:     domain.PageStatusDraft,
			Visibility: domain.PageVisibilityPublic,
			CreatedBy:  userID,
		})
		if err != nil {
			t.Fatalf("CreatePage: %v", err)
		}

		copyrightText := "© 2026 Footer Resolve Test"
		settings := domain.PageSettings{FooterCopyrightText: copyrightText}
		_, err = pageRepo.Update(ctx, tenants.A.Scope, footerPage.ID, repository.UpdatePageParams{
			Settings: &settings,
		})
		if err != nil {
			t.Fatalf("UpdatePage settings: %v", err)
		}

		_, err = sectionRepo.Create(ctx, tenants.A.Scope, repository.CreateSectionParams{
			LandingPageID: footerPage.ID,
			Key:           "footer-main",
			Type:          domain.SectionTypeFooter,
			Name:          "Footer",
			IsEnabled:     true,
			Content:       map[string]any{},
			Style:         map[string]any{},
		})
		if err != nil {
			t.Fatalf("CreateSection footer: %v", err)
		}

		reusableRepo := repository.NewReusableRepository(db)
		footerMenu, err := reusableRepo.CreateMenu(ctx, tenants.A.Scope, repository.CreateMenuParams{
			Name:      "Footer Menu",
			Location:  domain.MenuLocationFooter,
			IsActive:  true,
			CreatedBy: userID,
		})
		if err != nil {
			t.Fatalf("CreateMenu footer: %v", err)
		}
		t.Cleanup(func() {
			_ = reusableRepo.DeleteMenu(context.Background(), tenants.A.Scope, footerMenu.ID, userID)
		})
		_, err = reusableRepo.CreateMenuItem(ctx, tenants.A.Scope, repository.CreateMenuItemParams{
			MenuID:      footerMenu.ID,
			Label:       "Privacy",
			LinkType:    domain.LinkTypeInternalPage,
			Destination: "/privacy",
			Target:      domain.CTATargetSelf,
			SortOrder:   0,
			IsEnabled:   true,
		})
		if err != nil {
			t.Fatalf("CreateMenuItem footer: %v", err)
		}

		cta, err := reusableRepo.CreateCTA(ctx, tenants.A.Scope, repository.CreateCTAParams{
			Name:        "Footer Secondary CTA",
			Label:       "Talk to Sales",
			Type:        domain.CTATypeInternalPage,
			Target:      domain.CTATargetSelf,
			Destination: "/contact",
			TrackingKey: "footer-secondary-cta",
			CreatedBy:   userID,
		})
		if err != nil {
			t.Fatalf("CreateCTA footer: %v", err)
		}
		t.Cleanup(func() {
			_ = reusableRepo.DeleteCTA(context.Background(), tenants.A.Scope, cta.ID, userID)
		})

		if _, err := publishService.Publish(ctx, tenants.A.Scope, footerPage.ID, "Footer Release", userID); err != nil {
			t.Fatalf("Publish: %v", err)
		}

		resolved, err := resolverService.ResolveBySlug(ctx, tenants.A.Scope, footerSlug, "")
		if err != nil {
			t.Fatalf("ResolveBySlug: %v", err)
		}
		if resolved.Page.Settings.FooterCopyrightText != copyrightText {
			t.Errorf("Expected Page.Settings.FooterCopyrightText = %q, got %q", copyrightText, resolved.Page.Settings.FooterCopyrightText)
		}
		if resolved.Branding.ID == "" {
			t.Errorf("Expected Branding to be populated (default fallback if none set)")
		}
		foundFooterMenu := false
		for _, menu := range resolved.Menus {
			if menu.Location == string(domain.MenuLocationFooter) {
				foundFooterMenu = true
			}
		}
		if !foundFooterMenu {
			t.Errorf("Expected Menus to include a menu with location=footer, got %+v", resolved.Menus)
		}
		foundCTA := false
		for _, resolvedCTA := range resolved.CTAs {
			if resolvedCTA.TrackingKey == "footer-secondary-cta" && resolvedCTA.Label == "Talk to Sales" {
				foundCTA = true
			}
		}
		if !foundCTA {
			t.Errorf("Expected CTAs to include the secondary CTA by tracking key, got %+v", resolved.CTAs)
		}
	})
}

func TestResolverServiceGrapesJSIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	userID := "22222222-2222-2222-2222-222222222222"
	_, _ = db.Exec(ctx, `INSERT INTO users (id, name, email, status)
		VALUES ($1,'Mock User B','mock_b@example.com','active') ON CONFLICT DO NOTHING`, userID)
	_, _ = db.Exec(ctx, `INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1,'customer','organization-b','Organization B','active') ON CONFLICT DO NOTHING`, tenants.A.OrganizationID)

	pageRepo := repository.NewPageRepository(db)
	sectionRepo := repository.NewSectionRepository(db)
	formRepo := repository.NewFormRepository(db)
	brandingRepo := repository.NewBrandingRepository(db)
	versionRepo := repository.NewVersionRepository(db)
	resolverRepo := repository.NewResolverRepository(db)
	reusableRepo := repository.NewReusableRepository(db)
	documentRepo := repository.NewDocumentRepository(db)
	publishService := service.NewPublishService(db, pageRepo, sectionRepo, formRepo, brandingRepo, versionRepo, documentRepo, "test-secret")
	resolverService := service.NewResolverService(db, resolverRepo, versionRepo, pageRepo, sectionRepo, formRepo, brandingRepo, reusableRepo, documentRepo, publishService)

	slug := strings.ReplaceAll(testutil.UniqueCode("gjs-res-"), ".", "-")
	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name: "GJS Resolve", Title: "GJS Resolve", Slug: slug,
		Type: domain.PageTypeCampaign, Builder: domain.PageBuilderGrapesJS,
		Status: domain.PageStatusDraft, Visibility: domain.PageVisibilityPublic, CreatedBy: userID,
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if _, err := documentRepo.Upsert(ctx, tenants.A.Scope, repository.UpsertDocumentParams{
		LandingPageID: page.ID,
		Project:       map[string]any{"pages": []any{}},
		HTML:          `<main><h1>Live</h1></main>`,
		CSS:           `h1{color:blue}`,
		UpdatedBy:     userID,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Draft preview → live document, sanitized.
	token, _ := publishService.GeneratePreviewToken(ctx, tenants.A.Scope, page.ID, 10)
	preview, err := resolverService.ResolveBySlug(ctx, tenants.A.Scope, slug, token)
	if err != nil {
		t.Fatalf("ResolveBySlug (preview): %v", err)
	}
	if preview.Builder != string(domain.PageBuilderGrapesJS) || !strings.Contains(preview.HTML, "<h1>Live</h1>") {
		t.Fatalf("preview resolve = %+v", preview)
	}
	if len(preview.Sections) != 0 {
		t.Fatalf("grapesjs preview should carry no sections, got %d", len(preview.Sections))
	}

	// Published → served from the version snapshot.
	if _, err := publishService.Publish(ctx, tenants.A.Scope, page.ID, "release", userID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	pub, err := resolverService.ResolveBySlug(ctx, tenants.A.Scope, slug, "")
	if err != nil {
		t.Fatalf("ResolveBySlug (published): %v", err)
	}
	if pub.Builder != string(domain.PageBuilderGrapesJS) || !strings.Contains(pub.HTML, "<h1>Live</h1>") || !strings.Contains(pub.CSS, "color:blue") {
		t.Fatalf("published resolve = %+v", pub)
	}
}

// newTestUUID returns a random v4 UUID; the repo has no uuid dependency.
func newTestUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// newHomepageTenant creates an isolated organization (fresh id) so homepage
// tests never see flags left behind by other tests sharing the fixed tenants.
func newHomepageTenant(t *testing.T, db *database.Pool, orgType coretenant.OrganizationType) coretenant.Scope {
	t.Helper()
	ctx := context.Background()
	orgID := newTestUUID()
	slug := "hp-" + orgID[:8]
	if _, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, $2, $3, $3, 'active')`, orgID, string(orgType), slug); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), "DELETE FROM landing_pages WHERE organization_id = $1", orgID)
		_, _ = db.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", orgID)
	})
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     orgID,
		OrganizationSlug:   slug,
		OrganizationType:   orgType,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID:       newTestUUID(),
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext: %v", err)
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	return scope
}

type homepageEnv struct {
	db       *database.Pool
	pageRepo repository.PageRepository
	pageSvc  service.PageService
	resolver service.ResolverService
}

func newHomepageEnv(t *testing.T) homepageEnv {
	t.Helper()
	db := testutil.OpenTestDatabase(t)
	pageRepo := repository.NewPageRepository(db)
	sectionRepo := repository.NewSectionRepository(db)
	formRepo := repository.NewFormRepository(db)
	brandingRepo := repository.NewBrandingRepository(db)
	versionRepo := repository.NewVersionRepository(db)
	documentRepo := repository.NewDocumentRepository(db)
	publishService := service.NewPublishService(db, pageRepo, sectionRepo, formRepo, brandingRepo, versionRepo, documentRepo, "test-secret")
	return homepageEnv{
		db:       db,
		pageRepo: pageRepo,
		pageSvc:  service.NewPageService(pageRepo, sectionRepo),
		resolver: service.NewResolverService(db, repository.NewResolverRepository(db), versionRepo, pageRepo, sectionRepo, formRepo, brandingRepo, repository.NewReusableRepository(db), documentRepo, publishService),
	}
}

func (e homepageEnv) newPage(t *testing.T, scope coretenant.Scope, slug string, status domain.PageStatus, homepage bool) domain.LandingPage {
	t.Helper()
	page, err := e.pageRepo.Create(context.Background(), scope, repository.CreatePageParams{
		Name:       slug,
		Title:      slug,
		Slug:       slug,
		Type:       domain.PageTypeCampaign,
		Status:     status,
		Visibility: domain.PageVisibilityPublic,
		IsHomepage: homepage,
	})
	if err != nil {
		t.Fatalf("Create page %s: %v", slug, err)
	}
	return page
}

func (e homepageEnv) homepageIDs(t *testing.T, scope coretenant.Scope) []string {
	t.Helper()
	rows, err := e.db.Query(context.Background(),
		"SELECT id FROM landing_pages WHERE organization_id = $1 AND is_homepage AND deleted_at IS NULL ORDER BY id",
		scope.OrganizationID())
	if err != nil {
		t.Fatalf("query homepages: %v", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestPageUpdate_SetHomepageMovesFlag(t *testing.T) {
	env := newHomepageEnv(t)
	ctx := context.Background()
	scope := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)
	other := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)

	a := env.newPage(t, scope, "page-a", domain.PageStatusPublished, true)
	b := env.newPage(t, scope, "page-b", domain.PageStatusDraft, false)
	otherHome := env.newPage(t, other, "other-home", domain.PageStatusPublished, true)

	yes := true
	updated, err := env.pageSvc.Update(ctx, scope, b.ID, repository.UpdatePageParams{IsHomepage: &yes})
	if err != nil {
		t.Fatalf("Update is_homepage=true: %v", err)
	}
	if !updated.IsHomepage {
		t.Fatal("returned page should be homepage")
	}
	if ids := env.homepageIDs(t, scope); len(ids) != 1 || ids[0] != b.ID {
		t.Fatalf("homepages = %v, want only %s (a=%s)", ids, b.ID, a.ID)
	}
	// Another tenant's homepage is never touched.
	if ids := env.homepageIDs(t, other); len(ids) != 1 || ids[0] != otherHome.ID {
		t.Fatalf("other tenant homepages = %v, want %s", ids, otherHome.ID)
	}

	// Setting the current homepage again is a no-op, not a unique violation.
	if _, err := env.pageSvc.Update(ctx, scope, b.ID, repository.UpdatePageParams{IsHomepage: &yes}); err != nil {
		t.Fatalf("idempotent set: %v", err)
	}
}

func TestSetHomepage_ClearsArchivedFlagSoRestoreDoesNotCollide(t *testing.T) {
	env := newHomepageEnv(t)
	ctx := context.Background()
	scope := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)

	old := env.newPage(t, scope, "old-home", domain.PageStatusArchived, true) // outside the unique index
	fresh := env.newPage(t, scope, "fresh-home", domain.PageStatusPublished, false)

	if err := env.pageRepo.SetHomepage(ctx, scope, fresh.ID); err != nil {
		t.Fatalf("SetHomepage: %v", err)
	}
	if err := env.pageSvc.Restore(ctx, scope, old.ID, ""); err != nil {
		t.Fatalf("Restore archived ex-homepage must not collide: %v", err)
	}
	if ids := env.homepageIDs(t, scope); len(ids) != 1 || ids[0] != fresh.ID {
		t.Fatalf("homepages = %v, want only %s", ids, fresh.ID)
	}
}

func TestSetHomepage_InvalidTargetLeavesOldHomepageIntact(t *testing.T) {
	env := newHomepageEnv(t)
	ctx := context.Background()
	scope := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)
	other := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)

	home := env.newPage(t, scope, "home", domain.PageStatusPublished, true)
	deleted := env.newPage(t, scope, "deleted", domain.PageStatusDraft, false)
	if err := env.pageRepo.Delete(ctx, scope, deleted.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	tmpl, err := env.pageRepo.Create(ctx, scope, repository.CreatePageParams{
		Name: "tmpl", Title: "tmpl", Slug: "tmpl", Type: domain.PageTypeCampaign,
		Status: domain.PageStatusDraft, Visibility: domain.PageVisibilityPublic, IsTemplate: true,
	})
	if err != nil {
		t.Fatalf("Create template: %v", err)
	}
	foreign := env.newPage(t, other, "foreign", domain.PageStatusPublished, false)

	for name, id := range map[string]string{
		"nonexistent":  newTestUUID(),
		"soft-deleted": deleted.ID,
		"template":     tmpl.ID,
		"other-tenant": foreign.ID,
	} {
		if err := env.pageRepo.SetHomepage(ctx, scope, id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("%s: err = %v, want pgx.ErrNoRows", name, err)
		}
		if ids := env.homepageIDs(t, scope); len(ids) != 1 || ids[0] != home.ID {
			t.Fatalf("%s: homepages = %v, want only %s", name, ids, home.ID)
		}
	}

	// Via the service the same failure is a 404 and other fields are not applied.
	yes := true
	title := "should not apply"
	if _, err := env.pageSvc.Update(ctx, scope, newTestUUID(), repository.UpdatePageParams{IsHomepage: &yes, Title: &title}); err == nil {
		t.Fatal("expected error for unknown page")
	}
	if ids := env.homepageIDs(t, scope); len(ids) != 1 || ids[0] != home.ID {
		t.Fatalf("after service failure homepages = %v", ids)
	}
	if ids := env.homepageIDs(t, other); len(ids) != 0 {
		t.Fatalf("other tenant gained a homepage: %v", ids)
	}
}

func TestSetHomepage_ConcurrentCallsKeepSingleHomepage(t *testing.T) {
	env := newHomepageEnv(t)
	ctx := context.Background()
	scope := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)

	home := env.newPage(t, scope, "home", domain.PageStatusPublished, true)
	x := env.newPage(t, scope, "x", domain.PageStatusPublished, false)
	y := env.newPage(t, scope, "y", domain.PageStatusPublished, false)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{x.ID, y.ID} {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			errs[i] = env.pageRepo.SetHomepage(ctx, scope, id)
		}(i, id)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent SetHomepage #%d: %v", i, err)
		}
	}
	ids := env.homepageIDs(t, scope)
	if len(ids) != 1 || ids[0] == home.ID {
		t.Fatalf("homepages = %v, want exactly one of x/y", ids)
	}
}

func TestResolveHomepage_ReturnsPublishedHomepage(t *testing.T) {
	env := newHomepageEnv(t)
	scope := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)

	env.newPage(t, scope, "public-marketing", domain.PageStatusPublished, false)
	home := env.newPage(t, scope, "beranda", domain.PageStatusPublished, true)

	resolved, err := env.resolver.ResolveHomepage(context.Background(), scope)
	if err != nil {
		t.Fatalf("ResolveHomepage: %v", err)
	}
	if resolved.Page.ID != home.ID {
		t.Fatalf("resolved %s (%s), want homepage %s", resolved.Page.ID, resolved.Page.Slug, home.ID)
	}
}

func TestResolveHomepage_FallsBackToPublicMarketing(t *testing.T) {
	env := newHomepageEnv(t)
	ctx := context.Background()

	cases := map[string]domain.PageStatus{
		"draft":       domain.PageStatusDraft,
		"unpublished": domain.PageStatusUnpublished,
		"archived":    domain.PageStatusArchived,
	}
	for name, status := range cases {
		t.Run(name, func(t *testing.T) {
			scope := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)
			fallback := env.newPage(t, scope, "public-marketing", domain.PageStatusPublished, false)
			env.newPage(t, scope, "beranda", status, true)

			resolved, err := env.resolver.ResolveHomepage(ctx, scope)
			if err != nil {
				t.Fatalf("ResolveHomepage: %v", err)
			}
			if resolved.Page.ID != fallback.ID {
				t.Fatalf("resolved %s, want fallback %s", resolved.Page.Slug, fallback.Slug)
			}
			if resolved.IsDraft {
				t.Fatal("homepage resolve must never return a draft preview")
			}
		})
	}

	t.Run("no homepage at all", func(t *testing.T) {
		scope := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)
		fallback := env.newPage(t, scope, "public-marketing", domain.PageStatusPublished, false)
		resolved, err := env.resolver.ResolveHomepage(ctx, scope)
		if err != nil || resolved.Page.ID != fallback.ID {
			t.Fatalf("resolved = %+v err = %v", resolved.Page.Slug, err)
		}
	})
}

func TestResolveHomepage_NotFoundWhenNeither(t *testing.T) {
	env := newHomepageEnv(t)
	ctx := context.Background()

	t.Run("no pages", func(t *testing.T) {
		scope := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)
		if _, err := env.resolver.ResolveHomepage(ctx, scope); !errors.Is(err, service.ErrPageNotFound) {
			t.Fatalf("err = %v, want ErrPageNotFound", err)
		}
	})
	t.Run("homepage and fallback both unpublished", func(t *testing.T) {
		scope := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)
		env.newPage(t, scope, "public-marketing", domain.PageStatusDraft, false)
		env.newPage(t, scope, "beranda", domain.PageStatusArchived, true)
		_, err := env.resolver.ResolveHomepage(ctx, scope)
		if !errors.Is(err, service.ErrPageNotFound) && !errors.Is(err, service.ErrPageNotPublished) {
			t.Fatalf("err = %v, want not-found/not-published (both are 404 at the handler)", err)
		}
	})
}

func TestResolveHomepage_ScopeNeverReturnsAnotherOrganizationsHomepage(t *testing.T) {
	env := newHomepageEnv(t)
	ctx := context.Background()
	// Only one platform organization may exist (unique index), so two customer
	// tenants stand in for "platform vs tenant".
	platform := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)
	tenantScope := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)

	platformHome := env.newPage(t, platform, "beranda", domain.PageStatusPublished, true)
	tenantHome := env.newPage(t, tenantScope, "tenant-home", domain.PageStatusPublished, true)

	got, err := env.resolver.ResolveHomepage(ctx, tenantScope)
	if err != nil || got.Page.ID != tenantHome.ID {
		t.Fatalf("tenant resolve = %+v err=%v, want %s", got.Page.ID, err, tenantHome.ID)
	}
	got, err = env.resolver.ResolveHomepage(ctx, platform)
	if err != nil || got.Page.ID != platformHome.ID {
		t.Fatalf("first-org resolve = %+v err=%v, want %s", got.Page.ID, err, platformHome.ID)
	}

	// A tenant without any homepage/fallback must NOT leak the platform's page.
	empty := newHomepageTenant(t, env.db, coretenant.OrganizationTypeCustomer)
	if _, err := env.resolver.ResolveHomepage(ctx, empty); err == nil {
		t.Fatal("tenant without homepage must not resolve another organization's page")
	}
}
