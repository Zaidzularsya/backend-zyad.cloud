//go:build integration

package repository_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

func newAccount(t *testing.T, e routerEnv, scope coretenant.Scope) string {
	t.Helper()
	var acct string
	secretScopedTx(t, e, scope, func(tx pgx.Tx) {
		if err := tx.QueryRow(e.ctx, `INSERT INTO receivable_accounts (organization_id, name) VALUES ($1, 'Cust') RETURNING id`,
			scope.OrganizationID()).Scan(&acct); err != nil {
			t.Fatalf("insert account: %v", err)
		}
		if err := tx.Commit(e.ctx); err != nil {
			t.Fatal(err)
		}
	})
	return acct
}

func seedSecret(t *testing.T, e routerEnv, scope coretenant.Scope, routerID string, s repository.ImportedSecret) string {
	t.Helper()
	if _, err := secretRepo(e).UpsertImported(e.ctx, scope, routerID, []repository.ImportedSecret{s}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return findByName(t, e, scope, routerID, s.Name)
}

func TestSecretRepo_PairSetsFieldsAndOriginalProfile(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	id := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "alice", Profile: "paket"})
	acct := newAccount(t, e, scope)

	if err := secretRepo(e).Pair(e.ctx, scope, id, acct, ""); err != nil {
		t.Fatalf("pair: %v", err)
	}
	got, _ := secretRepo(e).FindByID(e.ctx, scope, id)
	if got.AccountID == nil || *got.AccountID != acct || got.PairedAt == nil || got.OriginalProfile != "paket" {
		t.Fatalf("got %+v", got)
	}
	if err := secretRepo(e).Pair(e.ctx, scope, id, acct, ""); !errors.Is(err, mikrotik.ErrSecretAlreadyPaired) {
		t.Fatalf("second pair err = %v", err)
	}
}

func TestSecretRepo_PairKeepsExistingOriginalProfile(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	id := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "alice", Profile: "isolir", NeedsOriginalProfile: true})
	repo := secretRepo(e)
	acct := newAccount(t, e, scope)
	if err := repo.Pair(e.ctx, scope, id, acct, ""); !errors.Is(err, mikrotik.ErrSecretNotPairable) {
		t.Fatalf("needs original profile: err = %v", err)
	}
	if err := repo.SetOriginalProfile(e.ctx, scope, id, "paket-gold"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Pair(e.ctx, scope, id, acct, ""); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.FindByID(e.ctx, scope, id)
	if got.OriginalProfile != "paket-gold" {
		t.Fatalf("original_profile = %q", got.OriginalProfile)
	}
}

func TestSecretRepo_PairRejections(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	repo := secretRepo(e)
	acct := newAccount(t, e, scope)

	if err := repo.Pair(e.ctx, scope, "00000000-0000-0000-0000-000000000001", acct, ""); !errors.Is(err, mikrotik.ErrSecretNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	id := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "gone", Profile: "p"})
	if _, err := repo.MarkMissing(e.ctx, scope, r.ID, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Pair(e.ctx, scope, id, acct, ""); !errors.Is(err, mikrotik.ErrSecretNotPairable) {
		t.Fatalf("missing: %v", err)
	}
	// Other organization cannot see/pair it.
	if err := repo.Pair(e.ctx, e.tenants.B.Scope, id, acct, ""); !errors.Is(err, mikrotik.ErrSecretNotFound) {
		t.Fatalf("cross-org: %v", err)
	}
}

func TestSecretRepo_PairForeignAccountRejectedByFK(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	id := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "alice", Profile: "p"})
	foreign := newAccount(t, e, e.tenants.B.Scope)
	if err := secretRepo(e).Pair(e.ctx, scope, id, foreign, ""); !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestPair_ConcurrentOnlyOneWins(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	id := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "alice", Profile: "p"})
	a1, a2 := newAccount(t, e, scope), newAccount(t, e, scope)

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for i, acct := range []string{a1, a2} {
		wg.Add(1)
		go func(i int, acct string) {
			defer wg.Done()
			<-start
			errs[i] = secretRepo(e).Pair(e.ctx, scope, id, acct, "")
		}(i, acct)
	}
	close(start)
	wg.Wait()

	wins, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, mikrotik.ErrSecretAlreadyPaired):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d, want 1/1", wins, conflicts)
	}
}

func TestSecretRepo_Unpair(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	repo := secretRepo(e)
	id := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "alice", Profile: "paket"})
	acct := newAccount(t, e, scope)
	if err := repo.Pair(e.ctx, scope, id, acct, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.Unpair(e.ctx, scope, id); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.FindByID(e.ctx, scope, id)
	if got.AccountID != nil || got.PairedAt != nil || got.PairedBy != nil || got.OriginalProfile != "paket" {
		t.Fatalf("got %+v", got)
	}

	if err := repo.Pair(e.ctx, scope, id, acct, ""); err != nil {
		t.Fatal(err)
	}
	secretScopedTx(t, e, scope, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_ppp_secrets SET isolation_state = 'isolated' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		_ = tx.Commit(e.ctx)
	})
	if err := repo.Unpair(e.ctx, scope, id); !errors.Is(err, mikrotik.ErrSecretIsolated) {
		t.Fatalf("isolated: %v", err)
	}
	if err := repo.Unpair(e.ctx, scope, "00000000-0000-0000-0000-000000000001"); !errors.Is(err, mikrotik.ErrSecretNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if err := repo.Unpair(e.ctx, e.tenants.B.Scope, id); !errors.Is(err, mikrotik.ErrSecretNotFound) {
		t.Fatalf("cross-org: %v", err)
	}
}
