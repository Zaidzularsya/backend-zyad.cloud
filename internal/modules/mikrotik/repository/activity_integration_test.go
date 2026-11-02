//go:build integration

package repository_test

import (
	"testing"
	"time"

	"zyad.cloud/internal/modules/mikrotik/repository"
	"zyad.cloud/internal/modules/mikrotik/service"
)

func actRepo(e routerEnv) repository.ActivityRepository {
	return repository.NewActivityRepository(e.db)
}

// insertAudit writes an audit row with an explicit time so ordering is deterministic.
func insertAudit(t *testing.T, e routerEnv, org, module, event string, at time.Time) {
	t.Helper()
	if _, err := e.db.Exec(e.ctx, `
		INSERT INTO audit_logs (module, event, target_type, metadata, organization_id, created_at)
		VALUES ($1, $2, 'mikrotik_router', '{"k":"v"}'::jsonb, $3, $4)`, module, event, org, at.UTC()); err != nil {
		t.Fatalf("insert audit: %v", err)
	}
}

func TestActivity_OnlyOwnOrgAndKnownEvents(t *testing.T) {
	e := setupRouter(t)
	a, b := e.tenants.A, e.tenants.B
	t.Cleanup(func() {
		_, _ = e.db.Exec(e.ctx, `DELETE FROM audit_logs WHERE organization_id = ANY($1::uuid[])`, []string{a.OrganizationID, b.OrganizationID})
	})
	now := time.Now()
	insertAudit(t, e, a.OrganizationID, "mikrotik", "pilot.added", now)
	insertAudit(t, e, a.OrganizationID, "mikrotik", "router.created", now)  // not allow-listed
	insertAudit(t, e, a.OrganizationID, "mikrotik", "secret.imported", now) // not allow-listed
	insertAudit(t, e, a.OrganizationID, "other", "pilot.added", now)        // other module
	insertAudit(t, e, b.OrganizationID, "mikrotik", "pilot.removed", now)   // other org

	rows, total, err := actRepo(e).ListActivity(e.ctx, a.Scope, service.ActivityEvents, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 || rows[0].Event != "pilot.added" || rows[0].Metadata["k"] != "v" {
		t.Fatalf("A rows=%+v total=%d", rows, total)
	}
	rows, total, err = actRepo(e).ListActivity(e.ctx, b.Scope, service.ActivityEvents, 50, 0)
	if err != nil || total != 1 || rows[0].Event != "pilot.removed" {
		t.Fatalf("B rows=%+v total=%d err=%v", rows, total, err)
	}
}

func TestActivity_PaginationOrder(t *testing.T) {
	e := setupRouter(t)
	a := e.tenants.A
	t.Cleanup(func() {
		_, _ = e.db.Exec(e.ctx, `DELETE FROM audit_logs WHERE organization_id = $1`, a.OrganizationID)
	})
	base := time.Now().Add(-time.Hour)
	events := []string{"pilot.added", "pilot.removed", "task.failed", "control.mode_changed", "task.retried"}
	for i, ev := range events {
		insertAudit(t, e, a.OrganizationID, "mikrotik", ev, base.Add(time.Duration(i)*time.Minute))
	}
	page1, total, err := actRepo(e).ListActivity(e.ctx, a.Scope, service.ActivityEvents, 2, 0)
	if err != nil || total != 5 || len(page1) != 2 {
		t.Fatalf("page1=%+v total=%d err=%v", page1, total, err)
	}
	page2, _, _ := actRepo(e).ListActivity(e.ctx, a.Scope, service.ActivityEvents, 2, 2)
	page3, _, _ := actRepo(e).ListActivity(e.ctx, a.Scope, service.ActivityEvents, 2, 4)
	got := []string{page1[0].Event, page1[1].Event, page2[0].Event, page2[1].Event, page3[0].Event}
	want := []string{"task.retried", "control.mode_changed", "task.failed", "pilot.removed", "pilot.added"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if len(page3) != 1 {
		t.Fatalf("page3 = %d", len(page3))
	}
}
