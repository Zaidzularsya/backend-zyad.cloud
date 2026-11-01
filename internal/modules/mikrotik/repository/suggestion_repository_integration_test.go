//go:build integration

package repository_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/mikrotik/repository"
)

func countSuggestions(t *testing.T, e routerEnv, routerID string) map[string]repository.Suggestion {
	t.Helper()
	scope := e.tenants.A.Scope
	out := map[string]repository.Suggestion{}
	secretScopedTx(t, e, scope, func(tx pgx.Tx) {
		rows, err := tx.Query(e.ctx, `SELECT s.secret_id, COALESCE(s.account_id::text, ''), s.basis, s.candidates
			FROM mikrotik_secret_suggestions s JOIN mikrotik_ppp_secrets p ON p.id = s.secret_id WHERE p.router_id = $1`, routerID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var s repository.Suggestion
			if err := rows.Scan(&s.SecretID, &s.AccountID, &s.Basis, &s.Candidates); err != nil {
				t.Fatal(err)
			}
			out[s.SecretID] = s
		}
	})
	return out
}

func TestSuggestionRepository_ReplaceForRouter(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r1 := e.create(t, scope, "R-sug-1")
	r2 := e.create(t, scope, "R-sug-2")
	s1 := seedSecret(t, e, scope, r1.ID, repository.ImportedSecret{Name: "alice", Profile: "p"})
	s2 := seedSecret(t, e, scope, r1.ID, repository.ImportedSecret{Name: "bob", Profile: "p"})
	s3 := seedSecret(t, e, scope, r2.ID, repository.ImportedSecret{Name: "carol", Profile: "p"})
	acct := newAccount(t, e, scope)
	repo := repository.NewSuggestionRepository(e.db)

	if err := repo.ReplaceForRouter(e.ctx, scope, r1.ID, []repository.Suggestion{
		{SecretID: s1, AccountID: acct, Basis: "phone", Candidates: 1},
		{SecretID: s2, Basis: "ambiguous", Candidates: 3},
	}); err != nil {
		t.Fatalf("replace 1: %v", err)
	}
	if err := repo.ReplaceForRouter(e.ctx, scope, r2.ID, []repository.Suggestion{{SecretID: s3, AccountID: acct, Basis: "name", Candidates: 1}}); err != nil {
		t.Fatalf("replace r2: %v", err)
	}
	got := countSuggestions(t, e, r1.ID)
	if len(got) != 2 || got[s1].AccountID != acct || got[s1].Basis != "phone" || got[s2].Basis != "ambiguous" || got[s2].AccountID != "" || got[s2].Candidates != 3 {
		t.Fatalf("after first replace: %+v", got)
	}

	// Replacing r1 overwrites only r1's rows; r2 is untouched.
	if err := repo.ReplaceForRouter(e.ctx, scope, r1.ID, []repository.Suggestion{{SecretID: s2, AccountID: acct, Basis: "name", Candidates: 1}}); err != nil {
		t.Fatalf("replace 2: %v", err)
	}
	got = countSuggestions(t, e, r1.ID)
	if len(got) != 1 || got[s2].Basis != "name" {
		t.Fatalf("after second replace: %+v", got)
	}
	if n := len(countSuggestions(t, e, r2.ID)); n != 1 {
		t.Fatalf("other router's rows changed: %d", n)
	}

	// A secret of another router is not inserted under this one.
	if err := repo.ReplaceForRouter(e.ctx, scope, r1.ID, []repository.Suggestion{{SecretID: s3, AccountID: acct, Basis: "name", Candidates: 1}}); err != nil {
		t.Fatal(err)
	}
	if n := len(countSuggestions(t, e, r1.ID)); n != 0 {
		t.Fatalf("foreign secret inserted: %d", n)
	}

	// Empty input clears.
	if err := repo.ReplaceForRouter(e.ctx, scope, r2.ID, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(countSuggestions(t, e, r2.ID)); n != 0 {
		t.Fatalf("not cleared: %d", n)
	}
}
