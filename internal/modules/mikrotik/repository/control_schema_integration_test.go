//go:build integration

package repository_test

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestControlSchema_DefaultsAndChecks(t *testing.T) {
	e := setupRouter(t)
	orgA := e.tenants.A.OrganizationID
	secretScopedTx(t, e, e.tenants.A.Scope, func(tx pgx.Tx) {
		var mode string
		var grace int
		if err := tx.QueryRow(e.ctx, `INSERT INTO mikrotik_org_settings (organization_id) VALUES ($1)
			RETURNING isolation_mode, grace_days`, orgA).Scan(&mode, &grace); err != nil {
			t.Fatalf("insert settings: %v", err)
		}
		if mode != "simulation" || grace != 0 {
			t.Fatalf("defaults = %q/%d, want simulation/0", mode, grace)
		}
		for name, q := range map[string]string{
			"bogus mode":  `UPDATE mikrotik_org_settings SET isolation_mode = 'bogus'`,
			"grace over":  `UPDATE mikrotik_org_settings SET grace_days = 61`,
			"grace negtv": `UPDATE mikrotik_org_settings SET grace_days = -1`,
		} {
			if _, err := tx.Exec(e.ctx, "SAVEPOINT s1"); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(e.ctx, q); err == nil {
				t.Fatalf("%s must be rejected", name)
			}
			if _, err := tx.Exec(e.ctx, "ROLLBACK TO SAVEPOINT s1"); err != nil {
				t.Fatal(err)
			}
		}
		for _, m := range []string{"pilot", "active", "paused"} {
			if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_org_settings SET isolation_mode = $1, grace_days = 60`, m); err != nil {
				t.Fatalf("mode %s: %v", m, err)
			}
		}
	})
}

func TestPolicyExemptedUntilColumn(t *testing.T) {
	e := setupRouter(t)
	acct := newAccount(t, e, e.tenants.A.Scope)
	secretScopedTx(t, e, e.tenants.A.Scope, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, `INSERT INTO mikrotik_customer_policies (organization_id, account_id, exempt, exempted_until)
			VALUES ($1, $2, true, DATE '2030-01-31')`, e.tenants.A.OrganizationID, acct); err != nil {
			t.Fatalf("exempted_until: %v", err)
		}
	})
}

func TestPilotAccounts_RLSAndCompositeFK(t *testing.T) {
	e := setupRouter(t)
	orgA := e.tenants.A.OrganizationID
	acctA := newAccount(t, e, e.tenants.A.Scope)
	acctB := newAccount(t, e, e.tenants.B.Scope)
	secretScopedTx(t, e, e.tenants.A.Scope, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, `INSERT INTO mikrotik_pilot_accounts (organization_id, account_id) VALUES ($1, $2)`, orgA, acctA); err != nil {
			t.Fatalf("insert pilot: %v", err)
		}
		if _, err := tx.Exec(e.ctx, "SAVEPOINT s1"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(e.ctx, `INSERT INTO mikrotik_pilot_accounts (organization_id, account_id) VALUES ($1, $2)`, orgA, acctB); err == nil {
			t.Fatal("pilot pointing to another org's account must be rejected")
		}
		if _, err := tx.Exec(e.ctx, "ROLLBACK TO SAVEPOINT s1"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(e.ctx, `INSERT INTO mikrotik_pilot_accounts (organization_id, account_id) VALUES ($1, $2)`, orgA, acctA); err == nil {
			t.Fatal("duplicate pilot must be rejected")
		}
		if _, err := tx.Exec(e.ctx, "ROLLBACK TO SAVEPOINT s1"); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := tx.QueryRow(e.ctx, "SELECT count(*) FROM mikrotik_pilot_accounts").Scan(&n); err != nil || n != 1 {
			t.Fatalf("org A must see its pilot: n=%d err=%v", n, err)
		}
		if _, err := tx.Exec(e.ctx, "SELECT set_config('app.organization_id', $1, true)", e.tenants.B.OrganizationID); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(e.ctx, "SELECT count(*) FROM mikrotik_pilot_accounts").Scan(&n); err != nil || n != 0 {
			t.Fatalf("org B must not see org A pilots: n=%d err=%v", n, err)
		}
	})
}

func TestControlPermissions_Grants(t *testing.T) {
	e := setupRouter(t)
	has := func(role, perm string) bool {
		var n int
		err := e.db.QueryRow(e.ctx, `
			SELECT count(*) FROM role_permissions rp
			JOIN roles r ON r.id = rp.role_id
			JOIN permissions p ON p.id = rp.permission_id
			WHERE (r.role_name = $1 OR r.slug = $1) AND p.permission_name = $2`, role, perm).Scan(&n)
		if err != nil {
			t.Fatalf("query %s: %v", perm, err)
		}
		return n > 0
	}
	if !has("member", "mikrotik_control.read") {
		t.Fatal("member must have mikrotik_control.read")
	}
	for _, p := range []string{"mikrotik_control.update", "mikrotik_control.restore"} {
		if has("member", p) {
			t.Fatalf("member must NOT have %s", p)
		}
	}
	for _, p := range []string{"mikrotik_control.read", "mikrotik_control.update", "mikrotik_control.restore"} {
		if !has("organization_owner", p) {
			t.Fatalf("owner must have %s", p)
		}
	}
}

func TestControlSchema_DownMigrations(t *testing.T) {
	e := setupRouter(t)
	tx, err := e.db.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(e.ctx)
	for _, f := range []string{"000177_mikrotik_control_permissions", "000176_mikrotik_control_center"} {
		sql, err := os.ReadFile("../../../../migrations/" + f + ".down.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(e.ctx, string(sql)); err != nil {
			t.Fatalf("run %s down: %v", f, err)
		}
	}
	var left int
	if err := tx.QueryRow(e.ctx, `SELECT (SELECT count(*) FROM permissions WHERE permission_name LIKE 'mikrotik_control.%')
		+ (SELECT count(*) FROM information_schema.tables WHERE table_name IN ('mikrotik_org_settings','mikrotik_pilot_accounts'))
		+ (SELECT count(*) FROM information_schema.columns WHERE table_name = 'mikrotik_customer_policies' AND column_name = 'exempted_until')`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("leftovers: %d %v", left, err)
	}
}
