//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
)

func setupOrganizations(t *testing.T, db *database.Pool, tenants testutil.TenantPair) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES
		($1, 'customer', 'organization-a', 'Organization A', 'active'),
		($2, 'customer', 'organization-b', 'Organization B', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID, tenants.B.OrganizationID)
	if err != nil {
		t.Fatalf("insert organizations: %v", err)
	}
	cleanup := func() {
		// wa_sessions is under FORCE RLS; delete per organization scope.
		for _, orgID := range []string{tenants.A.OrganizationID, tenants.B.OrganizationID} {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Errorf("cleanup begin: %v", err)
				return
			}
			_, _ = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID)
			_, _ = tx.Exec(ctx, "DELETE FROM wa_sessions WHERE organization_id = $1", orgID)
			_ = tx.Commit(ctx)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
}

func TestSessionRepositoryLifecycleAndIsolationIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	tenants := testutil.NewTenantPair(t)
	setupOrganizations(t, db, tenants)
	ctx := context.Background()
	sessions := repository.NewSessionRepository(db)
	directory := repository.NewDirectoryRepository(db)

	a1, err := sessions.Create(ctx, tenants.A.Scope, repository.CreateSessionParams{
		Name: "zc_test_a1", DisplayName: "Sales A", Engine: "GOWS",
		Purpose: domain.SessionPurposeSales, IsDefault: true,
	})
	if err != nil {
		t.Fatalf("Create A1: %v", err)
	}
	if a1.Status != domain.SessionStatusStopped || !a1.IsDefault || a1.WAHAServerID != "default" {
		t.Fatalf("A1 defaults = %+v", a1)
	}

	b1, err := sessions.Create(ctx, tenants.B.Scope, repository.CreateSessionParams{
		Name: "zc_test_b1", Purpose: domain.SessionPurposeCS,
	})
	if err != nil {
		t.Fatalf("Create B1: %v", err)
	}

	// Session names are global on the WAHA server.
	if _, err := sessions.Create(ctx, tenants.B.Scope, repository.CreateSessionParams{
		Name: "zc_test_a1", Purpose: domain.SessionPurposeSales,
	}); err == nil {
		t.Fatal("duplicate session name across organizations was accepted")
	}

	// A cannot see or change B.
	if _, err := sessions.GetByID(ctx, tenants.A.Scope, b1.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("A GetByID(B1) error = %v, want ErrNoRows", err)
	}
	name := "hijack"
	if _, err := sessions.Update(ctx, tenants.A.Scope, b1.ID, repository.UpdateSessionParams{DisplayName: &name}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("A Update(B1) error = %v, want ErrNoRows", err)
	}
	if err := sessions.SoftDelete(ctx, tenants.A.Scope, b1.ID, ""); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("A SoftDelete(B1) error = %v, want ErrNoRows", err)
	}
	listA, err := sessions.List(ctx, tenants.A.Scope)
	if err != nil || len(listA) != 1 || listA[0].ID != a1.ID {
		t.Fatalf("List A = %+v, %v", listA, err)
	}

	// Only one default per organization: making A2 default unsets A1.
	yes := true
	a2, err := sessions.Create(ctx, tenants.A.Scope, repository.CreateSessionParams{
		Name: "zc_test_a2", Purpose: domain.SessionPurposeNotification,
	})
	if err != nil {
		t.Fatalf("Create A2: %v", err)
	}
	if a2, err = sessions.Update(ctx, tenants.A.Scope, a2.ID, repository.UpdateSessionParams{IsDefault: &yes}); err != nil || !a2.IsDefault {
		t.Fatalf("Update A2 default = %+v, %v", a2, err)
	}
	if a1, err = sessions.GetByID(ctx, tenants.A.Scope, a1.ID); err != nil || a1.IsDefault {
		t.Fatalf("A1 still default after A2 became default: %+v, %v", a1, err)
	}
	if total, err := sessions.CountActive(ctx, tenants.A.Scope); err != nil || total != 2 {
		t.Fatalf("CountActive A = %d, %v; want 2", total, err)
	}

	// Status update keeps phone unless provided.
	phone, push := "6281234567890", "Sales"
	updated, previous, err := sessions.UpdateStatus(ctx, tenants.A.Scope, a1.ID, repository.UpdateSessionStatusParams{
		Status: domain.SessionStatusWorking, Phone: &phone, PushName: &push, At: time.Now().UTC(),
	})
	if err != nil || updated.Status != domain.SessionStatusWorking || updated.Phone != phone || updated.LastStatusAt == nil {
		t.Fatalf("UpdateStatus WORKING = %+v, %v", updated, err)
	}
	if previous != domain.SessionStatusStopped {
		t.Fatalf("previous status = %q, want STOPPED", previous)
	}
	updated, previous, err = sessions.UpdateStatus(ctx, tenants.A.Scope, a1.ID, repository.UpdateSessionStatusParams{
		Status: domain.SessionStatusFailed, At: time.Now().UTC(),
	})
	if err != nil || updated.Phone != phone || updated.PushName != push || previous != domain.SessionStatusWorking {
		t.Fatalf("UpdateStatus FAILED = %+v, previous %q, %v", updated, previous, err)
	}

	// Directory resolves without tenant context, across organizations.
	entry, err := directory.Resolve(ctx, "zc_test_b1")
	if err != nil || entry.OrganizationID != tenants.B.OrganizationID || entry.SessionID != b1.ID {
		t.Fatalf("Resolve B1 = %+v, %v", entry, err)
	}
	active, err := directory.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if !containsSession(active, "zc_test_a1", "zc_test_a2", "zc_test_b1") {
		t.Fatalf("ListActive = %+v", active)
	}

	// Soft delete hides the session and its directory entry.
	if err := sessions.SoftDelete(ctx, tenants.A.Scope, a1.ID, ""); err != nil {
		t.Fatalf("SoftDelete A1: %v", err)
	}
	if _, err := sessions.GetByID(ctx, tenants.A.Scope, a1.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("GetByID after delete error = %v", err)
	}
	if _, err := directory.Resolve(ctx, "zc_test_a1"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("Resolve after delete error = %v", err)
	}
	if total, err := sessions.CountActive(ctx, tenants.A.Scope); err != nil || total != 1 {
		t.Fatalf("CountActive A after delete = %d, %v; want 1", total, err)
	}
}

func containsSession(entries []domain.DirectoryEntry, names ...string) bool {
	seen := map[string]bool{}
	for _, entry := range entries {
		seen[entry.SessionName] = true
	}
	for _, name := range names {
		if !seen[name] {
			return false
		}
	}
	return true
}
