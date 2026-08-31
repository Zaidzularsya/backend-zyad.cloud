//go:build integration

package service_test

import (
	"context"
	"strings"
	"testing"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/platform/database/testutil"
	"zyad.cloud/internal/platform/redis"
)

func TestVisibilityServiceIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)

	_, err := db.Exec(ctx, `
		INSERT INTO users (id, name, email, status)
		VALUES 
		('11111111-1111-1111-1111-111111111111', 'Mock User A', 'mock_a@example.com', 'active'),
		('550e8400-e29b-41d4-a716-446655440000', 'Mock User B', 'mock_b@example.com', 'active')
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
	// we will pass nil to redis to test the main logic without rate limiting for now.
	var redisClient *redis.Client = nil

	cfg := config.Config{
		Auth: config.AuthConfig{
			Secret: "test-secret",
		},
	}

	pageRepo := repository.NewPageRepository(db)
	platformRepo := repository.NewPlatformPageRepository(db)
	visibilityService, err := service.NewVisibilityService(pageRepo, platformRepo, redisClient, cfg)
	if err != nil {
		t.Fatalf("failed to create visibility service: %v", err)
	}

	// Create a Page
	page, err := pageRepo.Create(ctx, tenants.A.Scope, repository.CreatePageParams{
		Name:       "Test Page",
		Title:      "Test Title",
		Slug:       strings.ReplaceAll(testutil.UniqueCode("vis-page-"), ".", "-"),
		Type:       domain.PageTypeCampaign,
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("Create page: %v", err)
	}

	t.Run("UpdateVisibility to PasswordProtected fails without password", func(t *testing.T) {
		err := visibilityService.UpdateVisibility(ctx, tenants.A.Scope, page.ID, domain.PageVisibilityPasswordProtected, "11111111-1111-1111-1111-111111111111")
		if err != service.ErrPasswordRequired {
			t.Errorf("expected ErrPasswordRequired, got %v", err)
		}
	})

	t.Run("SetPassword and VerifyAccess", func(t *testing.T) {
		err := visibilityService.SetPassword(ctx, tenants.A.Scope, page.ID, "mysecret123", "11111111-1111-1111-1111-111111111111")
		if err != nil {
			t.Fatalf("SetPassword: %v", err)
		}

		// Now update visibility
		err = visibilityService.UpdateVisibility(ctx, tenants.A.Scope, page.ID, domain.PageVisibilityPasswordProtected, "11111111-1111-1111-1111-111111111111")
		if err != nil {
			t.Fatalf("UpdateVisibility: %v", err)
		}

		// Verify with wrong password
		_, err = visibilityService.VerifyAccess(ctx, tenants.A.OrganizationID, page.ID, "wrong", "127.0.0.1")
		if err != service.ErrInvalidPassword {
			t.Errorf("expected ErrInvalidPassword, got %v", err)
		}

		// Verify with correct password
		token, err := visibilityService.VerifyAccess(ctx, tenants.A.OrganizationID, page.ID, "mysecret123", "127.0.0.1")
		if err != nil {
			t.Fatalf("VerifyAccess: %v", err)
		}
		if token == "" {
			t.Fatal("expected a token, got empty string")
		}

		// Validate the token
		err = visibilityService.ValidateAccessGrant(ctx, page.ID, token)
		if err != nil {
			t.Fatalf("ValidateAccessGrant: %v", err)
		}
	})

	t.Run("RemovePassword falls back to Private", func(t *testing.T) {
		err := visibilityService.RemovePassword(ctx, tenants.A.Scope, page.ID, "11111111-1111-1111-1111-111111111111")
		if err != nil {
			t.Fatalf("RemovePassword: %v", err)
		}

		updatedPage, err := pageRepo.FindByID(ctx, tenants.A.Scope, page.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}

		if updatedPage.Visibility != domain.PageVisibilityPrivate {
			t.Errorf("expected visibility private, got %s", updatedPage.Visibility)
		}
		if updatedPage.PasswordHash != "" {
			t.Errorf("expected empty password hash, got %s", updatedPage.PasswordHash)
		}
	})
}
