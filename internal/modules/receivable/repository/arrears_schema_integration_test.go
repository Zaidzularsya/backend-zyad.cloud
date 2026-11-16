//go:build integration

package repository_test

import (
	"os"
	"strings"
	"testing"
)

func TestArrearsSchema_UpCreatesTablesColumnsAndRLS(t *testing.T) {
	e := setup(t)
	for _, name := range []string{"receivable_arrears_recordings", "receivable_isolir_flag_audits"} {
		var rls, force bool
		if err := e.db.QueryRow(e.ctx, `SELECT relrowsecurity, relforcerowsecurity FROM pg_class WHERE oid = to_regclass($1)`,
			"public."+name).Scan(&rls, &force); err != nil {
			t.Fatalf("table %s: %v", name, err)
		}
		if !rls || !force {
			t.Fatalf("table %s must have forced RLS: rls=%v force=%v", name, rls, force)
		}
	}
	for _, col := range []string{"isolir_flag", "arrears_recording_id"} {
		var n int
		if err := e.db.QueryRow(e.ctx, `SELECT count(*) FROM information_schema.columns
			WHERE table_name = 'receivable_invoices' AND column_name = $1`, col).Scan(&n); err != nil || n != 1 {
			t.Fatalf("column receivable_invoices.%s: n=%d err=%v", col, n, err)
		}
	}
}

// No "admin" role is seeded; 000185 grants to it only where it exists (pattern of 000054),
// so the test creates one inside a transaction and re-runs the up script.
func TestArrearsSchema_PermissionGrants(t *testing.T) {
	e := setup(t)
	tx, err := e.db.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(e.ctx)
	if _, err := tx.Exec(e.ctx, `INSERT INTO roles (role_name, slug) VALUES ('admin', 'admin') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("insert admin role: %v", err)
	}
	up, err := os.ReadFile("../../../../migrations/000185_receivable_arrears_permissions.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	if _, err := tx.Exec(e.ctx, string(up)); err != nil {
		t.Fatalf("re-run 000185 up: %v", err)
	}
	q := `SELECT r.slug FROM permissions p
		JOIN role_permissions rp ON rp.permission_id = p.id JOIN roles r ON r.id = rp.role_id
		WHERE p.permission_name = $1 ORDER BY r.slug`
	cases := map[string]string{
		"invoice.record_arrears": "admin,organization_owner,super_admin",
		"invoice.isolir_flag":    "organization_owner,super_admin",
	}
	for perm, want := range cases {
		rows, err := tx.Query(e.ctx, q, perm)
		if err != nil {
			t.Fatal(err)
		}
		got := []string{}
		for rows.Next() {
			var s string
			_ = rows.Scan(&s)
			got = append(got, s)
		}
		rows.Close()
		if strings.Join(got, ",") != want {
			t.Fatalf("%s granted to %v, want %s", perm, got, want)
		}
	}
}

func TestArrearsSchema_DownDropsEverything(t *testing.T) {
	e := setup(t)
	tx, err := e.db.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(e.ctx)
	for _, f := range []string{"000185_receivable_arrears_permissions", "000184_receivable_arrears"} {
		sql, err := os.ReadFile("../../../../migrations/" + f + ".down.sql")
		if err != nil {
			t.Fatalf("read down migration: %v", err)
		}
		if _, err := tx.Exec(e.ctx, string(sql)); err != nil {
			t.Fatalf("run %s down: %v", f, err)
		}
	}
	for _, name := range []string{"receivable_arrears_recordings", "receivable_isolir_flag_audits"} {
		var reg *string
		if err := tx.QueryRow(e.ctx, "SELECT to_regclass($1)::text", "public."+name).Scan(&reg); err != nil {
			t.Fatal(err)
		}
		if reg != nil {
			t.Fatalf("table %s still exists after down", name)
		}
	}
	var cols, perms int
	_ = tx.QueryRow(e.ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name = 'receivable_invoices'
		AND column_name IN ('isolir_flag', 'arrears_recording_id')`).Scan(&cols)
	_ = tx.QueryRow(e.ctx, `SELECT count(*) FROM permissions WHERE permission_name IN ('invoice.record_arrears', 'invoice.isolir_flag')`).Scan(&perms)
	if cols != 0 || perms != 0 {
		t.Fatalf("left after down: columns=%d permissions=%d", cols, perms)
	}
}
