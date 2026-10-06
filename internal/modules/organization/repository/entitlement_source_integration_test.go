//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestEntitlementSourcesAccepted(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("entsrc"), ".", "-")

	organization, err := NewOrganizationRepository(db).Create(ctx, CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          "entsrc-" + suffix,
		Name:          "Entitlement Source Test",
		Status:        coretenant.OrganizationStatusActive,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM organization_entitlements WHERE organization_id = $1`, organization.ID)
		_, _ = db.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, organization.ID)
	})

	for _, source := range []string{"contract", "default"} {
		if _, err := db.Exec(ctx, `
			INSERT INTO organization_entitlements (organization_id, feature_key, source, limits)
			VALUES ($1, 'user.max', $2, '{}'::jsonb)`, organization.ID, source); err != nil {
			t.Fatalf("source %q should be accepted: %v", source, err)
		}
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO organization_entitlements (organization_id, feature_key, source, limits)
		VALUES ($1, 'user.max', 'foo', '{}'::jsonb)`, organization.ID); err == nil {
		t.Fatal("source foo should be rejected")
	}
}
