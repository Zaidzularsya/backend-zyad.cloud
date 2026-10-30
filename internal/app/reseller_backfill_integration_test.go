//go:build integration

package app

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	organizationmodel "zyad.cloud/internal/modules/organization/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestRunResellerBundleIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var orgID string
	if err := db.QueryRow(ctx, `
		INSERT INTO organizations (type, slug, name, status, data_placement)
		VALUES ('customer', $1, 'Reseller Backfill', 'active', 'shared') RETURNING id::text`,
		"rb-"+suffix).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM organization_entitlements WHERE organization_id = $1::uuid`, orgID)
		_, _ = db.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1::uuid`, orgID)
	})

	repo := organizationrepo.NewEntitlementRepository(db)
	if _, err := repo.Upsert(ctx, organizationrepo.UpsertEntitlementParams{
		OrganizationID: orgID, FeatureKey: featurePageActivation, Source: organizationmodel.EntitlementSourceContract,
		SourceReference: "11111111-2222-3333-4444-555555555555", Status: organizationmodel.EntitlementStatusActive,
		Limits: map[string]any{"value": true, "enabled": true}, EffectiveFrom: time.Now().UTC().Add(-time.Hour), Reason: "test",
	}); err != nil {
		t.Fatalf("seed page_activation: %v", err)
	}

	var dry bytes.Buffer
	if err := RunResellerBundle(ctx, db, false, &dry); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if !strings.Contains(dry.String(), "rb-"+suffix) || !strings.Contains(dry.String(), "DRY-RUN") {
		t.Fatalf("dry-run harus mencantumkan org uji:\n%s", dry.String())
	}
	var rows int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM organization_entitlements WHERE organization_id = $1::uuid AND feature_key = 'landing.max_pages'`, orgID).Scan(&rows)
	if rows != 0 {
		t.Fatalf("dry-run tidak boleh menulis, ada %d baris", rows)
	}

	var applied bytes.Buffer
	if err := RunResellerBundle(ctx, db, true, &applied); err != nil {
		t.Fatalf("apply: %v", err)
	}
	var limit string
	if err := db.QueryRow(ctx, `SELECT limits->>'value' FROM organization_entitlements
		WHERE organization_id = $1::uuid AND feature_key = 'landing.max_pages' AND status = 'active'`, orgID).Scan(&limit); err != nil || limit != "10" {
		t.Fatalf("landing.max_pages setelah apply = %q err=%v", limit, err)
	}
	var ref string
	_ = db.QueryRow(ctx, `SELECT source_reference FROM organization_entitlements WHERE organization_id = $1::uuid AND feature_key = 'storage.max_bytes'`, orgID).Scan(&ref)
	if ref != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("baris bundle harus memakai referensi contract yang sama, got %q", ref)
	}

	var again bytes.Buffer
	if err := RunResellerBundle(ctx, db, false, &again); err != nil {
		t.Fatalf("dry-run ulang: %v", err)
	}
	if strings.Contains(again.String(), "rb-"+suffix) {
		t.Fatalf("setelah apply org uji tidak boleh muncul lagi (idempoten):\n%s", again.String())
	}
}
