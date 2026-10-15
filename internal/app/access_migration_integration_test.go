//go:build integration

package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/platform/database/testutil"
)

// Review Focus 5: org platform tidak masuk daftar backfill maupun laporan preflight.
func TestAccessMigrationSkipsPlatformOrganizationIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var customerID, platformID string
	if err := db.QueryRow(ctx, `
		INSERT INTO organizations (type, slug, name, status, data_placement)
		VALUES ('customer', $1, 'Access Migration Customer', 'active', 'shared') RETURNING id::text`,
		"am-customer-"+suffix).Scan(&customerID); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	t.Cleanup(func() { db.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1::uuid`, customerID) })
	// Hanya satu org platform yang diizinkan (idx_organizations_single_platform): pakai yang ada,
	// buat bila DB uji belum punya.
	var platformSlug string
	err := db.QueryRow(ctx, `SELECT id::text, slug FROM organizations WHERE type = 'platform' AND deleted_at IS NULL LIMIT 1`).
		Scan(&platformID, &platformSlug)
	if err != nil {
		platformSlug = "am-platform-" + suffix
		if err := db.QueryRow(ctx, `
			INSERT INTO organizations (type, slug, name, status, data_placement)
			VALUES ('platform', $1, 'Access Migration Platform', 'active', 'shared') RETURNING id::text`,
			platformSlug).Scan(&platformID); err != nil {
			t.Fatalf("insert platform: %v", err)
		}
		t.Cleanup(func() { db.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1::uuid`, platformID) })
	}

	ids, err := listCustomerOrganizationIDs(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	foundCustomer := false
	for _, id := range ids {
		if id == platformID {
			t.Fatal("platform organization must not be listed for backfill")
		}
		if id == customerID {
			foundCustomer = true
		}
	}
	if !foundCustomer {
		t.Fatal("customer organization missing from backfill list")
	}

	report, err := RunPreflight(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if report.WorkspacesNeedingBackfill < 1 {
		t.Fatalf("report = %+v, want customer needing backfill", report)
	}
	for _, d := range report.Details {
		if strings.HasPrefix(d, platformSlug+":") {
			t.Fatalf("platform org in preflight details: %s", d)
		}
	}
}
