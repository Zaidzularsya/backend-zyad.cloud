//go:build integration

package service_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestPricingServiceIntegration(t *testing.T) {
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
	pricingService := service.NewPricingService(repo)
	userID := "11111111-1111-1111-1111-111111111111"

	t.Run("Create requires name and price label", func(t *testing.T) {
		_, err := pricingService.Create(ctx, tenants.A.Scope, repository.CreatePricingPlanParams{
			PriceLabel: "Rp 100.000",
			CreatedBy:  userID,
		})
		if err != service.ErrPricingPlanNameRequired {
			t.Fatalf("expected ErrPricingPlanNameRequired, got %v", err)
		}

		_, err = pricingService.Create(ctx, tenants.A.Scope, repository.CreatePricingPlanParams{
			Name:      "Starter",
			CreatedBy: userID,
		})
		if err != service.ErrPricingPlanPriceRequired {
			t.Fatalf("expected ErrPricingPlanPriceRequired, got %v", err)
		}
	})

	var planID string

	t.Run("Create plan", func(t *testing.T) {
		created, err := pricingService.Create(ctx, tenants.A.Scope, repository.CreatePricingPlanParams{
			Name:       "Starter",
			PriceLabel: "Rp 199.000",
			Features:   []string{"5 halaman", "Domain kustom"},
			IsEnabled:  true,
			CreatedBy:  userID,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if created.ID == "" {
			t.Fatalf("expected non-empty ID")
		}
		if created.CTALabel != "Pilih paket" {
			t.Errorf("expected default CTA label, got %q", created.CTALabel)
		}
		if len(created.Features) != 2 {
			t.Errorf("expected 2 features, got %d", len(created.Features))
		}
		planID = created.ID
	})

	t.Run("Update plan", func(t *testing.T) {
		newName := "Starter Plus"
		updated, err := pricingService.Update(ctx, tenants.A.Scope, planID, repository.UpdatePricingPlanParams{
			Name:      &newName,
			UpdatedBy: userID,
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Name != newName {
			t.Errorf("expected name %q, got %q", newName, updated.Name)
		}
	})

	t.Run("List plans", func(t *testing.T) {
		list, err := pricingService.List(ctx, tenants.A.Scope)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) < 1 {
			t.Errorf("expected at least 1 plan, got %d", len(list))
		}
	})

	t.Run("Delete plan", func(t *testing.T) {
		if err := pricingService.Delete(ctx, tenants.A.Scope, planID, userID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		_, err := pricingService.FindByID(ctx, tenants.A.Scope, planID)
		if err == nil {
			t.Errorf("expected error after delete, got nil")
		}
	})
}
