//go:build integration

package service_test

import (
	"context"
	"strings"
	"testing"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
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
