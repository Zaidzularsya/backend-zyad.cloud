//go:build integration

package app

import (
	"context"
	"testing"
	"time"

	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
)

func TestContactMeterExcludesDeletedRecords(t *testing.T) {
	db, scope, reg := meterFixture(t)
	registerCoreQuotaMeters(reg, db)
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		tx, err := db.Begin(cctx)
		if err != nil {
			return
		}
		defer tx.Rollback(cctx)
		_, _ = tx.Exec(cctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID())
		_, _ = tx.Exec(cctx, "DELETE FROM crm_contacts WHERE organization_id = $1", scope.OrganizationID())
		_ = tx.Commit(cctx)
	})

	repo := crmrepo.NewContactRepository(db)
	var ids []string
	for _, name := range []string{"A", "B", "C"} {
		c, err := repo.Create(context.Background(), scope, crmrepo.CreateContactParams{FirstName: name})
		if err != nil {
			t.Fatalf("create contact: %v", err)
		}
		ids = append(ids, c.ID)
	}
	if got := meterUsed(t, reg, scope, crmdomain.FeatureCRMMaxContacts); got != 3 {
		t.Fatalf("Used = %d, want 3", got)
	}
	// The meter must equal what ContactService.requireCreateQuota counts.
	enforced, err := repo.Count(context.Background(), scope, crmrepo.ContactListFilter{})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if enforced != 3 {
		t.Fatalf("repo Count = %d, want 3", enforced)
	}
	if err := repo.Delete(context.Background(), scope, ids[0], ""); err != nil {
		t.Fatalf("delete contact: %v", err)
	}
	if got := meterUsed(t, reg, scope, crmdomain.FeatureCRMMaxContacts); got != 2 {
		t.Fatalf("Used after delete = %d, want 2", got)
	}
}

func TestStorageMeterSumsAssetObjectsAndLandingMedia(t *testing.T) {
	db, scope, reg := meterFixture(t)
	registerCoreQuotaMeters(reg, db)
	ctx := context.Background()
	orgID := scope.OrganizationID()

	mustExec := func(query string, args ...any) {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID); err != nil {
			t.Fatalf("set_config: %v", err)
		}
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			t.Fatalf("exec: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
	// asset_objects: 100 live + 40 deleted; landing_media_assets: 25 live + 5 deleted.
	mustExec(`INSERT INTO asset_objects (organization_id, storage_key, filename, mime_type, size_bytes) VALUES ($1, 'k1', 'a.png', 'image/png', 100)`, orgID)
	mustExec(`INSERT INTO asset_objects (organization_id, storage_key, filename, mime_type, size_bytes, deleted_at) VALUES ($1, 'k2', 'b.png', 'image/png', 40, now())`, orgID)
	mustExec(`INSERT INTO landing_media_assets (organization_id, storage_key, filename, mime_type, size_bytes) VALUES ($1, 'm1', 'c.png', 'image/png', 25)`, orgID)
	mustExec(`INSERT INTO landing_media_assets (organization_id, storage_key, filename, mime_type, size_bytes, deleted_at) VALUES ($1, 'm2', 'd.png', 'image/png', 5, now())`, orgID)

	if got := meterUsed(t, reg, scope, featureStorageMaxBytes); got != 125 {
		t.Fatalf("Used = %d, want 125", got)
	}
}
