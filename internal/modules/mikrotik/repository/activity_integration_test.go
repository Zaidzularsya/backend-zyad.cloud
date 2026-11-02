//go:build integration

package repository_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"

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

func insertAuditMD(t *testing.T, e routerEnv, org, event, md string, at time.Time) {
	t.Helper()
	if _, err := e.db.Exec(e.ctx, `
		INSERT INTO audit_logs (module, event, target_type, metadata, organization_id, created_at)
		VALUES ('mikrotik', $1, 'mikrotik_router', $2::jsonb, $3, $4)`, event, md, org, at.UTC()); err != nil {
		t.Fatalf("insert audit: %v", err)
	}
}

func cleanupAudit(t *testing.T, e routerEnv) {
	t.Cleanup(func() {
		_, _ = e.db.Exec(e.ctx, `DELETE FROM audit_logs WHERE organization_id = ANY($1::uuid[])`,
			[]string{e.tenants.A.OrganizationID, e.tenants.B.OrganizationID})
	})
}

func insertSecret(t *testing.T, e routerEnv, scope coretenant.Scope, routerID, name, account string) string {
	t.Helper()
	var id string
	secretScopedTx(t, e, scope, func(tx pgx.Tx) {
		if err := tx.QueryRow(e.ctx, `
			INSERT INTO mikrotik_ppp_secrets (organization_id, router_id, name, profile, disabled, isolation_state, account_id)
			VALUES ($1, $2, $3, 'default', false, 'normal', $4) RETURNING id::text`,
			scope.OrganizationID(), routerID, name, account).Scan(&id); err != nil {
			t.Fatalf("insert secret: %v", err)
		}
		if err := tx.Commit(e.ctx); err != nil {
			t.Fatal(err)
		}
	})
	return id
}

func TestActivity_NonObjectMetadataDoesNotFailPage(t *testing.T) {
	e := setupRouter(t)
	a := e.tenants.A
	cleanupAudit(t, e)
	now := time.Now()
	insertAuditMD(t, e, a.OrganizationID, "pilot.added", `[1,2]`, now)
	insertAuditMD(t, e, a.OrganizationID, "pilot.removed", `"text"`, now.Add(-time.Minute))
	insertAuditMD(t, e, a.OrganizationID, "task.failed", `{"a":1}`, now.Add(-2*time.Minute))
	rows, total, err := actRepo(e).ListActivity(e.ctx, a.Scope, service.ActivityEvents, 50, 0)
	if err != nil || total != 3 || len(rows) != 3 {
		t.Fatalf("rows=%d total=%d err=%v", len(rows), total, err)
	}
	if rows[0].Metadata != nil || rows[2].Metadata["a"] != float64(1) {
		t.Fatalf("metadata = %v / %v", rows[0].Metadata, rows[2].Metadata)
	}
}

func TestActivity_SecretJoin(t *testing.T) {
	e := setupRouter(t)
	a, b := e.tenants.A, e.tenants.B
	cleanupAudit(t, e)
	routerA := e.create(t, a.Scope, "R-A")
	routerB := e.create(t, b.Scope, "R-B")
	acctA := newAccount(t, e, a.Scope)
	secA := insertSecret(t, e, a.Scope, routerA.ID, "pppoe-a", acctA)
	secB := insertSecret(t, e, b.Scope, routerB.ID, "pppoe-b", newAccount(t, e, b.Scope))
	now := time.Now()
	// (d) old-style row: only secret_id.
	insertAuditMD(t, e, a.OrganizationID, "isolation.isolated", `{"secret_id":"`+secA+`"}`, now)
	// (b) foreign-org secret id must not leak name/account.
	insertAuditMD(t, e, a.OrganizationID, "isolation.restored", `{"secret_id":"`+secB+`"}`, now.Add(-time.Minute))
	// (c) malformed secret id must not error.
	insertAuditMD(t, e, a.OrganizationID, "task.failed", `{"secret_id":"abc"}`, now.Add(-2*time.Minute))

	rows, _, err := actRepo(e).ListActivity(e.ctx, a.Scope, service.ActivityEvents, 50, 0)
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	if rows[0].SecretName != "pppoe-a" || rows[0].SecretAccountID != acctA {
		t.Errorf("join row = %+v", rows[0])
	}
	if rows[1].SecretName != "" || rows[1].SecretAccountID != "" {
		t.Errorf("foreign secret leaked: %+v", rows[1])
	}
	if rows[2].SecretName != "" || rows[2].SecretAccountID != "" {
		t.Errorf("malformed secret_id row: %+v", rows[2])
	}
}

func TestActivity_LimitClampAndOffset(t *testing.T) {
	e := setupRouter(t)
	a := e.tenants.A
	cleanupAudit(t, e)
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 105; i++ {
		insertAudit(t, e, a.OrganizationID, "mikrotik", "pilot.added", base.Add(time.Duration(i)*time.Second))
	}
	svc := service.NewActivityService(actRepo(e), nil)
	items, total, err := svc.List(e.ctx, a.Scope, 1000, -5)
	if err != nil || len(items) != 100 || total != 105 {
		t.Fatalf("items=%d total=%d err=%v", len(items), total, err)
	}
	items, _, _ = svc.List(e.ctx, a.Scope, 0, 100)
	if len(items) != 5 {
		t.Fatalf("default limit/offset 100: items=%d", len(items))
	}
}

func TestActivity_QueryUsesIndex(t *testing.T) {
	e := setupRouter(t)
	a, b := e.tenants.A, e.tenants.B
	cleanupAudit(t, e)
	for _, org := range []struct {
		id string
		n  int
	}{{a.OrganizationID, 3000}, {b.OrganizationID, 5000}} {
		if _, err := e.db.Exec(e.ctx, `
			INSERT INTO audit_logs (module, event, target_type, metadata, organization_id, created_at)
			SELECT 'mikrotik', (ARRAY['pilot.added','task.failed','router.created'])[1 + g % 3], 'mikrotik_router', '{}'::jsonb, $1,
			       now() - (g || ' seconds')::interval
			FROM generate_series(1, $2) g`, org.id, org.n); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if _, err := e.db.Exec(e.ctx, `ANALYZE audit_logs`); err != nil {
		t.Fatal(err)
	}
	var plan []string
	secretScopedTx(t, e, a.Scope, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			t.Fatal(err)
		}
		rows, err := tx.Query(e.ctx, `EXPLAIN `+repository.ActivityListSQL,
			a.OrganizationID, service.ActivityEvents, 50, 0)
		if err != nil {
			t.Fatalf("explain: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var l string
			_ = rows.Scan(&l)
			plan = append(plan, l)
		}
	})
	joined := strings.Join(plan, "\n")
	t.Log("plan:\n" + joined)
	if !strings.Contains(joined, "idx_audit_logs_mikrotik_org_created") || strings.Contains(joined, "Seq Scan on audit_logs") {
		t.Fatalf("list query does not use the activity index:\n%s", joined)
	}
}
