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

func TestBrandingServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	brandingRepo := repository.NewBrandingRepository(db)
	brandingService := service.NewBrandingService(brandingRepo)

	defCompName := "Default Company"
	defLogo := "http://example.com/logo.png"

	// 1. Set Default Branding
	defBranding, err := brandingService.UpsertDefault(ctx, tenants.A.Scope, repository.CreateBrandingParams{
		CompanyName:  &defCompName,
		LogoLightURL: &defLogo,
		Colors: &domain.BrandingColors{
			Primary: "#FF0000",
		},
	})
	if err != nil {
		t.Fatalf("UpsertDefault: %v", err)
	}

	// 2. Override Page Branding
	pageRepo := repository.NewPageRepository(db)
	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Branding Page",
		Title:      "Branding Title",
		Slug:       strings.ReplaceAll(testutil.UniqueCode("branding-page-1"), ".", "-"),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPublic,
	})
	if err != nil {
		t.Fatalf("Create page: %v", err)
	}

	pageCompName := "Campaign Company"
	overrideBranding, err := brandingService.UpsertPageOverride(ctx, tenants.A.Scope, page.ID, repository.CreateBrandingParams{
		CompanyName: &pageCompName,
		Colors: &domain.BrandingColors{
			Primary: "#00FF00",
		},
	})
	if err != nil {
		t.Fatalf("UpsertPageOverride: %v", err)
	}

	// 3. Get Effective Branding
	effective, err := brandingService.GetEffectiveBranding(ctx, tenants.A.Scope, page.ID)
	if err != nil {
		t.Fatalf("GetEffectiveBranding: %v", err)
	}

	if effective.CompanyName != "Campaign Company" {
		t.Errorf("Expected CompanyName to be 'Campaign Company', got %v", effective.CompanyName)
	}
	if effective.LogoLightURL != "http://example.com/logo.png" {
		t.Errorf("Expected LogoLightURL to fallback to default, got %v", effective.LogoLightURL)
	}
	if effective.Colors.Primary != "#00FF00" {
		t.Errorf("Expected Primary color to be overridden to '#00FF00', got %v", effective.Colors.Primary)
	}

	// 4. Remove Override
	err = brandingService.RemovePageOverride(ctx, tenants.A.Scope, page.ID)
	if err != nil {
		t.Fatalf("RemovePageOverride: %v", err)
	}

	effectiveRemoved, _ := brandingService.GetEffectiveBranding(ctx, tenants.A.Scope, page.ID)
	if effectiveRemoved.CompanyName != "Default Company" {
		t.Errorf("Expected CompanyName to revert to 'Default Company', got %v", effectiveRemoved.CompanyName)
	}
	
	// Clean up default
	_, _ = defBranding, overrideBranding
}
