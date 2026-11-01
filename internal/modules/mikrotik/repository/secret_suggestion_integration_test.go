//go:build integration

package repository_test

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

const insertSuggestionSQL = `
	INSERT INTO mikrotik_secret_suggestions (organization_id, secret_id, account_id, basis, candidates)
	VALUES ($1, $2, $3, $4, $5)`

func TestSecretSuggestions_RLSAndCascade(t *testing.T) {
	e := setupRouter(t)
	orgA, orgB := e.tenants.A.OrganizationID, e.tenants.B.OrganizationID
	r := e.create(t, e.tenants.A.Scope, "R-sugg")

	var secretID string
	secretScopedTx(t, e, e.tenants.A.Scope, func(tx pgx.Tx) {
		if err := tx.QueryRow(e.ctx, insertSecretSQL+" RETURNING id", orgA, r.ID, "sugg-alice", "normal").Scan(&secretID); err != nil {
			t.Fatalf("insert secret: %v", err)
		}
		// ambiguous requires account_id NULL and candidates >= 2.
		if _, err := tx.Exec(e.ctx, "SAVEPOINT s1"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(e.ctx, insertSuggestionSQL, orgA, secretID, nil, "ambiguous", 1); err == nil {
			t.Fatal("ambiguous with candidates=1 must be rejected")
		}
		if _, err := tx.Exec(e.ctx, "ROLLBACK TO SAVEPOINT s1"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(e.ctx, insertSuggestionSQL, orgA, secretID, nil, "bogus", 1); err == nil {
			t.Fatal("unknown basis must be rejected")
		}
		if _, err := tx.Exec(e.ctx, "ROLLBACK TO SAVEPOINT s1"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(e.ctx, insertSuggestionSQL, orgA, secretID, nil, "ambiguous", 3); err != nil {
			t.Fatalf("insert suggestion: %v", err)
		}
		var n int
		if err := tx.QueryRow(e.ctx, "SELECT count(*) FROM mikrotik_secret_suggestions").Scan(&n); err != nil || n != 1 {
			t.Fatalf("org A must see its suggestion: n=%d err=%v", n, err)
		}
		// Switch scope to org B inside the same transaction: row must be invisible.
		if _, err := tx.Exec(e.ctx, "SELECT set_config('app.organization_id', $1, true)", orgB); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(e.ctx, "SELECT count(*) FROM mikrotik_secret_suggestions").Scan(&n); err != nil || n != 0 {
			t.Fatalf("org B must not see org A suggestions: n=%d err=%v", n, err)
		}
		if _, err := tx.Exec(e.ctx, "SELECT set_config('app.organization_id', $1, true)", orgA); err != nil {
			t.Fatal(err)
		}
		// Deleting the secret cascades to the suggestion.
		if _, err := tx.Exec(e.ctx, "DELETE FROM mikrotik_ppp_secrets WHERE id = $1", secretID); err != nil {
			t.Fatalf("delete secret: %v", err)
		}
		if err := tx.QueryRow(e.ctx, "SELECT count(*) FROM mikrotik_secret_suggestions").Scan(&n); err != nil || n != 0 {
			t.Fatalf("suggestion must cascade-delete with secret: n=%d err=%v", n, err)
		}
	})
}

func TestMemberHasTaskRead(t *testing.T) {
	e := setupRouter(t)
	has := func(perm string) bool {
		var n int
		err := e.db.QueryRow(e.ctx, `
			SELECT count(*) FROM role_permissions rp
			JOIN roles r ON r.id = rp.role_id
			JOIN permissions p ON p.id = rp.permission_id
			WHERE (r.role_name = 'member' OR r.slug = 'member') AND p.permission_name = $1`, perm).Scan(&n)
		if err != nil {
			t.Fatalf("query %s: %v", perm, err)
		}
		return n > 0
	}
	if !has("mikrotik_task.read") {
		t.Fatal("member must have mikrotik_task.read")
	}
	if has("mikrotik_task.retry") {
		t.Fatal("member must NOT have mikrotik_task.retry")
	}
}
