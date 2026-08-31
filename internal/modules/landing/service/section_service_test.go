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

func TestSectionServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	_, err := db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES 
		('11111111-1111-1111-1111-111111111111', 'Mock User A', 'mock_a@example.com', 'active')
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		t.Fatalf("failed to insert mock users: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES 
		($1, 'customer', 'organization-a', 'Organization A', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID)
	if err != nil {
		t.Fatalf("failed to insert mock organizations: %v", err)
	}

	pageRepo := repository.NewPageRepository(db)
	sectionRepo := repository.NewSectionRepository(db)
	sectionSvc := service.NewSectionService(sectionRepo)

	// Create Page
	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Page For Section Test",
		Title:      "Promo",
		Slug:       strings.ReplaceAll(testutil.UniqueCode("sec-page"), ".", "-"),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
		CreatedBy:  "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create page: %v", err)
	}

	// 1. Create Section with XSS
	sec, err := sectionSvc.Create(ctx, tenants.A.Scope, tenants.A.Context.OrganizationType(), repository.CreateSectionParams{
		LandingPageID: page.ID,
		Key:           "hero-1",
		Type:          domain.SectionTypeHero,
		Name:          "Hero Section",
		SortOrder:     0,
		IsEnabled:     true,
		Content: map[string]any{
			"title":    "Welcome <script>alert(1)</script>",
			"desc":     "Click <a href='javascript:alert(1)'>here</a>",
			"imgAttr":  "<img src=x onerror=alert(1)>",
			"svgAttr":  "<svg onload=alert(1)>",
			"safeLink": "Click <a href=\"https://example.com\">here</a>",
			"nested": []any{
				map[string]any{"text": "<iframe src='bad.com'></iframe> nested"},
			},
		},
		Style:     map[string]any{"color": "blue"},
		CreatedBy: "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("Create section: %v", err)
	}

	// Verify sanitization
	if sec.Content["title"] != "Welcome " {
		t.Errorf("Expected sanitized title, got: %v", sec.Content["title"])
	}
	if sec.Content["desc"] != "Click here" {
		t.Errorf("Expected sanitized desc, got: %v", sec.Content["desc"])
	}
	if sec.Content["imgAttr"] != "" {
		t.Errorf("Expected img onerror payload stripped entirely, got: %v", sec.Content["imgAttr"])
	}
	if sec.Content["svgAttr"] != "" {
		t.Errorf("Expected svg onload payload stripped entirely, got: %v", sec.Content["svgAttr"])
	}
	if sec.Content["safeLink"] != `Click <a href="https://example.com" rel="nofollow">here</a>` {
		t.Errorf("Expected safe link to be preserved with nofollow, got: %v", sec.Content["safeLink"])
	}
	nestedArr := sec.Content["nested"].([]any)
	nestedMap := nestedArr[0].(map[string]any)
	if nestedMap["text"] != " nested" {
		t.Errorf("Expected sanitized nested text, got: %v", nestedMap["text"])
	}

	// 2. Toggle
	err = sectionSvc.Toggle(ctx, tenants.A.Scope, sec.ID, false, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("Toggle section: %v", err)
	}
	secToggled, _ := sectionSvc.Get(ctx, tenants.A.Scope, sec.ID)
	if secToggled.IsEnabled {
		t.Error("Expected section to be disabled")
	}

	// 3. Delete
	err = sectionSvc.Delete(ctx, tenants.A.Scope, sec.ID)
	if err != nil {
		t.Fatalf("Delete section: %v", err)
	}
}
