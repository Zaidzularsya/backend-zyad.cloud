//go:build integration

package repository_test

import (
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const insertTaskSQL = `
	INSERT INTO mikrotik_isolation_tasks (organization_id, secret_id, action, status, created_by_event)
	VALUES ($1, $2, 'isolate', $3, 'test')`

// insertTestSecret inserts a PPP secret for the given organization and returns its id.
func insertTestSecret(t *testing.T, e routerEnv, orgID, routerID, name string, tx pgx.Tx) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(e.ctx, `
		INSERT INTO mikrotik_ppp_secrets (organization_id, router_id, name, profile)
		VALUES ($1, $2, $3, 'default') RETURNING id`, orgID, routerID, name).Scan(&id); err != nil {
		t.Fatalf("insert secret: %v", err)
	}
	return id
}

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func TestTaskSchema_OneActiveTaskPerSecret(t *testing.T) {
	e := setupRouter(t)
	r := e.create(t, e.tenants.A.Scope, "R1")
	secretScopedTx(t, e, e.tenants.A.Scope, func(tx pgx.Tx) {
		sid := insertTestSecret(t, e, e.tenants.A.OrganizationID, r.ID, "alice", tx)
		if _, err := tx.Exec(e.ctx, insertTaskSQL, e.tenants.A.OrganizationID, sid, "done"); err != nil {
			t.Fatalf("done task: %v", err)
		}
		if _, err := tx.Exec(e.ctx, insertTaskSQL, e.tenants.A.OrganizationID, sid, "pending"); err != nil {
			t.Fatalf("done + pending must be allowed: %v", err)
		}
		_, err := tx.Exec(e.ctx, insertTaskSQL, e.tenants.A.OrganizationID, sid, "pending")
		if sqlState(err) != "23505" {
			t.Fatalf("second pending must fail with 23505, got %v", err)
		}
	})
}

func TestTaskSchema_CrossOrgSecretRejected(t *testing.T) {
	e := setupRouter(t)
	r := e.create(t, e.tenants.A.Scope, "R1")
	var sid string
	tx, err := e.db.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(e.ctx)
	if _, err := tx.Exec(e.ctx, "SELECT set_config('app.organization_id', $1, true)", e.tenants.A.OrganizationID); err != nil {
		t.Fatal(err)
	}
	sid = insertTestSecret(t, e, e.tenants.A.OrganizationID, r.ID, "alice", tx)
	if _, err := tx.Exec(e.ctx, "SELECT set_config('app.organization_id', $1, true)", e.tenants.B.OrganizationID); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(e.ctx, insertTaskSQL, e.tenants.B.OrganizationID, sid, "pending")
	if sqlState(err) != "23503" {
		t.Fatalf("cross-org secret must fail with 23503, got %v", err)
	}
}

func TestTaskSchema_RLS(t *testing.T) {
	e := setupRouter(t)
	r := e.create(t, e.tenants.A.Scope, "R1")
	tx, err := e.db.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(e.ctx)
	if _, err := tx.Exec(e.ctx, "SELECT set_config('app.organization_id', $1, true)", e.tenants.A.OrganizationID); err != nil {
		t.Fatal(err)
	}
	sid := insertTestSecret(t, e, e.tenants.A.OrganizationID, r.ID, "alice", tx)
	if _, err := tx.Exec(e.ctx, insertTaskSQL, e.tenants.A.OrganizationID, sid, "pending"); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	var n int
	if err := tx.QueryRow(e.ctx, "SELECT count(*) FROM mikrotik_isolation_tasks").Scan(&n); err != nil || n != 1 {
		t.Fatalf("org A must see its task: n=%d err=%v", n, err)
	}
	if _, err := tx.Exec(e.ctx, "SELECT set_config('app.organization_id', $1, true)", e.tenants.B.OrganizationID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(e.ctx, "SELECT count(*) FROM mikrotik_isolation_tasks").Scan(&n); err != nil || n != 0 {
		t.Fatalf("org B must not see org A tasks: n=%d err=%v", n, err)
	}
}

func TestTaskSchema_PermissionsAndTemplate(t *testing.T) {
	e := setupRouter(t)
	var total, owners int
	if err := e.db.QueryRow(e.ctx, `SELECT count(DISTINCT p.id), count(*) FILTER (WHERE r.slug = 'organization_owner')
		FROM permissions p LEFT JOIN role_permissions rp ON rp.permission_id = p.id LEFT JOIN roles r ON r.id = rp.role_id
		WHERE p.permission_name IN ('mikrotik_task.read','mikrotik_task.retry')`).Scan(&total, &owners); err != nil {
		t.Fatal(err)
	}
	if total != 2 || owners < 2 {
		t.Fatalf("total=%d owners=%d", total, owners)
	}
	var tpl int
	if err := e.db.QueryRow(e.ctx, `SELECT count(*) FROM notification_templates WHERE code = 'mikrotik.isolation_task_failed' AND is_system`).Scan(&tpl); err != nil || tpl != 1 {
		t.Fatalf("template count=%d err=%v", tpl, err)
	}
}

func TestTaskSchema_DownMigrations(t *testing.T) {
	e := setupRouter(t)
	tx, err := e.db.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(e.ctx)
	for _, f := range []string{"000174_mikrotik_isolation_permissions_notification", "000173_create_mikrotik_isolation_tasks"} {
		sql, err := os.ReadFile("../../../../migrations/" + f + ".down.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(e.ctx, string(sql)); err != nil {
			t.Fatalf("run %s down: %v", f, err)
		}
	}
	var reg *string
	if err := tx.QueryRow(e.ctx, "SELECT to_regclass('public.mikrotik_isolation_tasks')::text").Scan(&reg); err != nil || reg != nil {
		t.Fatalf("tasks table still exists: %v %v", reg, err)
	}
	var left int
	if err := tx.QueryRow(e.ctx, `SELECT (SELECT count(*) FROM permissions WHERE permission_name LIKE 'mikrotik_task.%')
		+ (SELECT count(*) FROM notification_templates WHERE code = 'mikrotik.isolation_task_failed')`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("leftovers: %d %v", left, err)
	}
	var c int
	if err := tx.QueryRow(e.ctx, `SELECT count(*) FROM pg_constraint WHERE conname = 'mikrotik_ppp_secrets_org_id_unique'`).Scan(&c); err != nil || c != 0 {
		t.Fatalf("unique constraint left: %d %v", c, err)
	}
}
