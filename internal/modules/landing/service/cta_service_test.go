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

func TestCTAServiceIntegration(t *testing.T) {
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

	repo := repository.NewReusableRepository(db)
	ctaService := service.NewCTAService(repo)

	// User ID for mock
	userID := "11111111-1111-1111-1111-111111111111"

	var cta domain.LandingCTA

	t.Run("Create CTA", func(t *testing.T) {
		params := repository.CreateCTAParams{
			Name:        "Test CTA",
			Label:       "Click Here",
			Type:        domain.CTATypeExternalLink,
			Target:      domain.CTATargetNewTab,
			Destination: "https://example.com",
			TrackingKey: strings.ReplaceAll(testutil.UniqueCode("cta-key-"), ".", "-"),
			CreatedBy:   userID,
		}

		created, err := ctaService.Create(ctx, tenants.A.Scope, params)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if created.ID == "" {
			t.Errorf("expected non-empty ID")
		}
		if created.Name != params.Name {
			t.Errorf("expected Name %s, got %s", params.Name, created.Name)
		}

		cta = created
	})

	t.Run("FindByID CTA", func(t *testing.T) {
		found, err := ctaService.FindByID(ctx, tenants.A.Scope, cta.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if found.ID != cta.ID {
			t.Errorf("expected ID %s, got %s", cta.ID, found.ID)
		}
	})

	t.Run("Update CTA", func(t *testing.T) {
		newLabel := "Click Me Now"
		params := repository.UpdateCTAParams{
			Label:     &newLabel,
			UpdatedBy: userID,
		}

		updated, err := ctaService.Update(ctx, tenants.A.Scope, cta.ID, params)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}

		if updated.Label != newLabel {
			t.Errorf("expected Label %s, got %s", newLabel, updated.Label)
		}
	})

	t.Run("List CTAs", func(t *testing.T) {
		list, err := ctaService.List(ctx, tenants.A.Scope)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) < 1 {
			t.Errorf("expected at least 1 CTA, got %d", len(list))
		}
	})

	t.Run("Delete CTA", func(t *testing.T) {
		err := ctaService.Delete(ctx, tenants.A.Scope, cta.ID, userID)
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}

		// Should not be found after deletion
		_, err = ctaService.FindByID(ctx, tenants.A.Scope, cta.ID)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
	})
}
