//go:build integration

package repository_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/mikrotik/repository"
)

// A suggestion computed before a pairing is stale once the secret is paired
// and unpaired again; it must not resurface (the FE would pre-select it).
func TestSecretRepo_PairAndUnpairDropStaleSuggestion(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R-stale")
	sec := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "stale", Profile: "p"})
	suggested := newAccount(t, e, scope)
	actual := newAccount(t, e, scope)
	sugRepo := repository.NewSuggestionRepository(e.db)
	if err := sugRepo.ReplaceForRouter(e.ctx, scope, r.ID, []repository.Suggestion{{SecretID: sec, AccountID: suggested, Basis: "phone", Candidates: 1}}); err != nil {
		t.Fatal(err)
	}
	repo := secretRepo(e)
	count := func(f repository.SecretFilter) int64 {
		f.RouterID = r.ID
		_, total, err := repo.List(e.ctx, scope, f, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		return total
	}
	if n := count(repository.SecretFilter{Suggestion: repository.SuggestionFilterSuggested}); n != 1 {
		t.Fatalf("precondition: suggested = %d", n)
	}
	if err := repo.Pair(e.ctx, scope, sec, actual, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.Unpair(e.ctx, scope, sec); err != nil {
		t.Fatal(err)
	}
	if n := count(repository.SecretFilter{Suggestion: repository.SuggestionFilterSuggested}); n != 0 {
		t.Fatalf("stale suggestion resurfaced in suggested filter: %d", n)
	}
	items, _, err := repo.List(e.ctx, scope, repository.SecretFilter{RouterID: r.ID}, 50, 0)
	if err != nil || len(items) != 1 || items[0].Suggestion != nil {
		t.Fatalf("unfiltered list: err=%v items=%+v", err, items)
	}
	var rows int
	secretScopedTx(t, e, scope, func(tx pgx.Tx) {
		if err := tx.QueryRow(e.ctx, `SELECT count(*) FROM mikrotik_secret_suggestions WHERE secret_id = $1`, sec).Scan(&rows); err != nil {
			t.Fatal(err)
		}
	})
	if rows != 0 {
		t.Fatalf("suggestion rows left = %d", rows)
	}
}

// Unfiltered listing must not expose a hint for a secret that vanished from the router.
func TestSecretRepo_ListHidesSuggestionOfAbsentSecret(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R-absent")
	sec := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "absent", Profile: "p"})
	acct := newAccount(t, e, scope)
	if err := repository.NewSuggestionRepository(e.db).ReplaceForRouter(e.ctx, scope, r.ID, []repository.Suggestion{{SecretID: sec, AccountID: acct, Basis: "phone", Candidates: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := secretRepo(e).MarkAbsent(e.ctx, scope, sec); err != nil {
		t.Fatal(err)
	}
	items, _, err := secretRepo(e).List(e.ctx, scope, repository.SecretFilter{RouterID: r.ID}, 50, 0)
	if err != nil || len(items) != 1 || items[0].Suggestion != nil {
		t.Fatalf("err=%v items=%+v", err, items)
	}
}

// Secrets of a soft-deleted router are invisible to onboarding counters,
// customer status and the secrets list (the isolation engine ignores them too).
func TestSecretRepo_DeletedRouterSecretsAreExcluded(t *testing.T) {
	e := setupRouter(t)
	cleanPolicyRows(t, e)
	scope := e.tenants.A.Scope
	live := e.create(t, scope, "R-live")
	dead := e.create(t, scope, "R-dead")
	acctOnlyDead := newAccount(t, e, scope)
	acctBoth := newAccount(t, e, scope)
	pair := func(routerID, name, acct string) {
		sid := seedSecret(t, e, scope, routerID, repository.ImportedSecret{Name: name, Profile: "p"})
		if err := secretRepo(e).Pair(e.ctx, scope, sid, acct, ""); err != nil {
			t.Fatal(err)
		}
	}
	pair(live.ID, "live-1", acctBoth)
	pair(dead.ID, "dead-1", acctBoth)
	pair(dead.ID, "dead-2", acctOnlyDead)
	seedSecret(t, e, scope, dead.ID, repository.ImportedSecret{Name: "dead-needs", Profile: "iso", NeedsOriginalProfile: true})

	repo := onboardingRepo(e)
	before, err := repo.Counts(e.ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.repo.SoftDelete(e.ctx, scope, dead.ID, ""); err != nil {
		t.Fatal(err)
	}
	after, err := repo.Counts(e.ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if d := before.SecretsTotal - after.SecretsTotal; d != 3 {
		t.Errorf("secrets_total dropped by %d, want 3", d)
	}
	if d := before.SecretsPaired - after.SecretsPaired; d != 2 {
		t.Errorf("secrets_paired dropped by %d, want 2", d)
	}
	if d := before.NeedsProfile - after.NeedsProfile; d != 1 {
		t.Errorf("needs_profile dropped by %d, want 1", d)
	}

	st, err := repo.AccountStatuses(e.ctx, scope, []string{acctOnlyDead, acctBoth})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st[acctOnlyDead]; ok {
		t.Errorf("account whose only secret is on a deleted router must have no status: %+v", st[acctOnlyDead])
	}
	if st[acctBoth].Secrets != 1 {
		t.Errorf("acctBoth secrets = %d, want 1", st[acctBoth].Secrets)
	}

	items, total, err := secretRepo(e).List(e.ctx, scope, repository.SecretFilter{}, 200, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.RouterID == dead.ID {
			t.Fatalf("list returned a secret of a deleted router: %+v", it)
		}
	}
	if int(total) != len(items) {
		t.Errorf("total=%d items=%d", total, len(items))
	}
}
