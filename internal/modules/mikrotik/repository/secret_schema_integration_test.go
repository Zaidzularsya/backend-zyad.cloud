//go:build integration

package repository_test

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
)

func secretScopedTx(t *testing.T, e routerEnv, scope coretenant.Scope, fn func(tx pgx.Tx)) {
	t.Helper()
	tx, err := e.db.Begin(e.ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(e.ctx)
	if _, err := tx.Exec(e.ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID()); err != nil {
		t.Fatalf("set scope: %v", err)
	}
	fn(tx)
}

const insertSecretSQL = `
	INSERT INTO mikrotik_ppp_secrets (organization_id, router_id, name, profile, disabled, isolation_state)
	VALUES ($1, $2, $3, 'default', false, $4)`

func TestSecretSchema_UniqueRouterName(t *testing.T) {
	e := setupRouter(t)
	r := e.create(t, e.tenants.A.Scope, "R1")
	secretScopedTx(t, e, e.tenants.A.Scope, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, insertSecretSQL, e.tenants.A.OrganizationID, r.ID, "alice", "normal"); err != nil {
			t.Fatalf("first insert: %v", err)
		}
		if _, err := tx.Exec(e.ctx, insertSecretSQL, e.tenants.A.OrganizationID, r.ID, "alice", "normal"); err == nil {
			t.Fatal("duplicate (router_id, name) must be rejected")
		}
	})
}

func TestSecretSchema_RLSIsolatesOrganizations(t *testing.T) {
	e := setupRouter(t)
	r := e.create(t, e.tenants.A.Scope, "R1")
	tx, err := e.db.Begin(e.ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(e.ctx)
	if _, err := tx.Exec(e.ctx, "SELECT set_config('app.organization_id', $1, true)", e.tenants.A.OrganizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(e.ctx, insertSecretSQL, e.tenants.A.OrganizationID, r.ID, "alice", "normal"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var n int
	if err := tx.QueryRow(e.ctx, "SELECT count(*) FROM mikrotik_ppp_secrets").Scan(&n); err != nil || n != 1 {
		t.Fatalf("org A must see its row: n=%d err=%v", n, err)
	}
	if _, err := tx.Exec(e.ctx, "SELECT set_config('app.organization_id', $1, true)", e.tenants.B.OrganizationID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(e.ctx, "SELECT count(*) FROM mikrotik_ppp_secrets").Scan(&n); err != nil || n != 0 {
		t.Fatalf("org B must not see org A rows: n=%d err=%v", n, err)
	}
}

func TestSecretSchema_IsolationStateCheck(t *testing.T) {
	e := setupRouter(t)
	r := e.create(t, e.tenants.A.Scope, "R1")
	secretScopedTx(t, e, e.tenants.A.Scope, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, insertSecretSQL, e.tenants.A.OrganizationID, r.ID, "bob", "x"); err == nil {
			t.Fatal("isolation_state 'x' must be rejected")
		}
	})
}

func TestSecretSchema_DownMigrationDropsTables(t *testing.T) {
	e := setupRouter(t)
	tables := []string{"mikrotik_ppp_secrets", "mikrotik_customer_policies", "mikrotik_product_triggers"}
	// DDL is transactional in PostgreSQL: run the down scripts, assert, then roll back.
	tx, err := e.db.Begin(e.ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(e.ctx)
	for _, f := range []string{"000172_mikrotik_secret_permissions", "000171_create_mikrotik_secret_mapping"} {
		sql, err := os.ReadFile("../../../../migrations/" + f + ".down.sql")
		if err != nil {
			t.Fatalf("read down migration: %v", err)
		}
		if _, err := tx.Exec(e.ctx, string(sql)); err != nil {
			t.Fatalf("run %s down: %v", f, err)
		}
	}
	for _, name := range tables {
		var reg *string
		if err := tx.QueryRow(e.ctx, "SELECT to_regclass($1)::text", "public."+name).Scan(&reg); err != nil {
			t.Fatal(err)
		}
		if reg != nil {
			t.Fatalf("table %s still exists after down", name)
		}
	}
	var left int
	if err := tx.QueryRow(e.ctx, `SELECT count(*) FROM permissions WHERE permission_name ~ '^mikrotik_(secret|policy|product_trigger)\.'`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("permissions left after down: %d err=%v", left, err)
	}
}

func TestSecretSchema_PermissionsGrantedToOwnerOnly(t *testing.T) {
	e := setupRouter(t)
	var total, owners, others int
	q := `SELECT count(DISTINCT p.id),
		count(*) FILTER (WHERE r.slug = 'organization_owner'),
		count(*) FILTER (WHERE r.slug <> 'organization_owner')
		FROM permissions p LEFT JOIN role_permissions rp ON rp.permission_id = p.id LEFT JOIN roles r ON r.id = rp.role_id
		WHERE p.permission_name ~ '^mikrotik_(secret|policy|product_trigger)\.'`
	if err := e.db.QueryRow(e.ctx, q).Scan(&total, &owners, &others); err != nil {
		t.Fatal(err)
	}
	if total != 8 || others != 0 || owners < 8 {
		t.Fatalf("total=%d owners=%d others=%d", total, owners, others)
	}
}
