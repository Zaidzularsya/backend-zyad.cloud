//go:build integration

package repository_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

func secretRepo(e routerEnv) repository.SecretRepository { return repository.NewSecretRepository(e.db) }

func pair(t *testing.T, e routerEnv, scope coretenant.Scope, secretID string) {
	t.Helper()
	secretScopedTx(t, e, scope, func(tx pgx.Tx) {
		var acct string
		if err := tx.QueryRow(e.ctx, `INSERT INTO receivable_accounts (organization_id, name) VALUES ($1, 'Cust') RETURNING id`,
			scope.OrganizationID()).Scan(&acct); err != nil {
			t.Fatalf("insert account: %v", err)
		}
		if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_ppp_secrets SET account_id = $1, original_profile = 'paket', isolation_state = 'isolated' WHERE id = $2`,
			acct, secretID); err != nil {
			t.Fatalf("pair: %v", err)
		}
		if err := tx.Commit(e.ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	})
}

func findByName(t *testing.T, e routerEnv, scope coretenant.Scope, routerID, name string) (id string) {
	t.Helper()
	items, _, err := secretRepo(e).List(e.ctx, scope, repository.SecretFilter{RouterID: routerID, Search: name}, 50, 0)
	if err != nil || len(items) == 0 {
		t.Fatalf("find %s: %v (n=%d)", name, err, len(items))
	}
	return items[0].ID
}

func TestSecretRepo_UpsertIdempotentAndKeepsPairing(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	repo := secretRepo(e)
	t0 := time.Now().UTC().Truncate(time.Microsecond)

	res, err := repo.UpsertImported(e.ctx, scope, r.ID, []repository.ImportedSecret{
		{Name: "alice", Profile: "paket"}, {Name: "bob", Profile: "isolir", NeedsOriginalProfile: true}, {Name: "alice", Profile: "paket", Comment: "dup, last wins"},
	}, t0)
	if err != nil || res.Created != 2 || res.Updated != 0 {
		t.Fatalf("first upsert: %+v %v", res, err)
	}
	aliceID := findByName(t, e, scope, r.ID, "alice")
	got, err := repo.FindByID(e.ctx, scope, aliceID)
	if err != nil || got.Comment != "dup, last wins" || !got.Present || got.LastSeenAt == nil {
		t.Fatalf("alice: %+v %v", got, err)
	}
	bob, _ := repo.FindByID(e.ctx, scope, findByName(t, e, scope, r.ID, "bob"))
	if !bob.NeedsOriginalProfile {
		t.Fatal("bob must be flagged on insert")
	}

	pair(t, e, scope, aliceID)
	t1 := t0.Add(time.Minute)
	res, err = repo.UpsertImported(e.ctx, scope, r.ID, []repository.ImportedSecret{
		{Name: "alice", Profile: "isolir", Disabled: true, NeedsOriginalProfile: true}, {Name: "bob", Profile: "paket"},
	}, t1)
	if err != nil || res.Created != 0 || res.Updated != 2 {
		t.Fatalf("second upsert: %+v %v", res, err)
	}
	got, _ = repo.FindByID(e.ctx, scope, aliceID)
	if got.Profile != "isolir" || !got.Disabled {
		t.Fatalf("router-owned columns not updated: %+v", got)
	}
	if got.AccountID == nil || got.OriginalProfile != "paket" || got.IsolationState != "isolated" {
		t.Fatalf("pairing/isolation touched: %+v", got)
	}
	if got.NeedsOriginalProfile {
		t.Fatal("flag must not be raised when original_profile is already known")
	}
	bob, _ = repo.FindByID(e.ctx, scope, bob.ID)
	if !bob.NeedsOriginalProfile {
		t.Fatal("flag must never be auto-cleared")
	}
}

func TestSecretRepo_UpsertBatchesOver500(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	rows := make([]repository.ImportedSecret, 1203)
	for i := range rows {
		rows[i] = repository.ImportedSecret{Name: fmt.Sprintf("u%04d", i), Profile: "p"}
	}
	res, err := secretRepo(e).UpsertImported(e.ctx, scope, r.ID, rows, time.Now().UTC())
	if err != nil || res.Created != 1203 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	_, total, err := secretRepo(e).List(e.ctx, scope, repository.SecretFilter{RouterID: r.ID}, 10, 0)
	if err != nil || total != 1203 {
		t.Fatalf("total=%d err=%v", total, err)
	}
}

func TestSecretRepo_MarkMissing(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r1, r2 := e.create(t, scope, "R1"), e.create(t, scope, "R2")
	repo := secretRepo(e)
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	for _, id := range []string{r1.ID, r2.ID} {
		if _, err := repo.UpsertImported(e.ctx, scope, id, []repository.ImportedSecret{{Name: "alice"}, {Name: "bob"}}, t0); err != nil {
			t.Fatal(err)
		}
	}
	t1 := t0.Add(time.Minute)
	if _, err := repo.UpsertImported(e.ctx, scope, r1.ID, []repository.ImportedSecret{{Name: "alice"}}, t1); err != nil {
		t.Fatal(err)
	}
	n, err := repo.MarkMissing(e.ctx, scope, r1.ID, t1)
	if err != nil || n != 1 {
		t.Fatalf("missing=%d err=%v", n, err)
	}
	if n, _ = repo.MarkMissing(e.ctx, scope, r1.ID, t1); n != 0 {
		t.Fatalf("second MarkMissing must be a no-op, got %d", n)
	}
	bob, _ := repo.FindByID(e.ctx, scope, findByName(t, e, scope, r1.ID, "bob"))
	if bob.Present {
		t.Fatal("bob must be present=false")
	}
	other, _ := repo.FindByID(e.ctx, scope, findByName(t, e, scope, r2.ID, "bob"))
	if !other.Present {
		t.Fatal("other router must be untouched")
	}
	// Re-appearing secret becomes present again.
	if _, err := repo.UpsertImported(e.ctx, scope, r1.ID, []repository.ImportedSecret{{Name: "bob"}}, t1.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if bob, _ = repo.FindByID(e.ctx, scope, bob.ID); !bob.Present {
		t.Fatal("bob must be present again")
	}
}

func TestSecretRepo_RLSAcrossOrganizations(t *testing.T) {
	e := setupRouter(t)
	a, b := e.tenants.A.Scope, e.tenants.B.Scope
	r := e.create(t, a, "R1")
	repo := secretRepo(e)
	if _, err := repo.UpsertImported(e.ctx, a, r.ID, []repository.ImportedSecret{{Name: "alice"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	id := findByName(t, e, a, r.ID, "alice")
	if _, err := repo.FindByID(e.ctx, b, id); !errors.Is(err, mikrotik.ErrSecretNotFound) {
		t.Fatalf("org B FindByID err = %v, want ErrSecretNotFound", err)
	}
	if _, total, err := repo.List(e.ctx, b, repository.SecretFilter{}, 10, 0); err != nil || total != 0 {
		t.Fatalf("org B list total=%d err=%v", total, err)
	}
	if err := repo.SetOriginalProfile(e.ctx, b, id, "x"); !errors.Is(err, mikrotik.ErrSecretNotFound) {
		t.Fatalf("org B SetOriginalProfile err = %v", err)
	}
	// Writing into another organization's router must be rejected.
	if _, err := repo.UpsertImported(e.ctx, b, r.ID, []repository.ImportedSecret{{Name: "evil"}}, time.Now().UTC()); err == nil {
		t.Fatal("org B must not insert secrets for org A's router")
	}
}

func TestSecretRepo_ListFilterPaired(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	repo := secretRepo(e)
	if _, err := repo.UpsertImported(e.ctx, scope, r.ID, []repository.ImportedSecret{
		{Name: "alice", Comment: "100%_off"}, {Name: "bob"}, {Name: "carol"},
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	pair(t, e, scope, findByName(t, e, scope, r.ID, "alice"))

	yes, no := true, false
	_, total, _ := repo.List(e.ctx, scope, repository.SecretFilter{RouterID: r.ID, Paired: &yes}, 10, 0)
	if total != 1 {
		t.Fatalf("paired total = %d", total)
	}
	items, total, _ := repo.List(e.ctx, scope, repository.SecretFilter{RouterID: r.ID, Paired: &no}, 1, 0)
	if total != 2 || len(items) != 1 || items[0].Name != "bob" {
		t.Fatalf("unpaired: total=%d items=%+v", total, items)
	}
	if _, total, _ = repo.List(e.ctx, scope, repository.SecretFilter{RouterID: r.ID, Search: "100%_"}, 10, 0); total != 1 {
		t.Fatalf("literal search total = %d", total)
	}
	if _, total, _ = repo.List(e.ctx, scope, repository.SecretFilter{RouterID: r.ID, Search: "%"}, 10, 0); total != 1 {
		t.Fatalf("lone %% must match only the row containing a literal %%, total = %d", total)
	}
}

func TestSecretRepo_SetOriginalProfileClearsFlag(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	repo := secretRepo(e)
	if _, err := repo.UpsertImported(e.ctx, scope, r.ID, []repository.ImportedSecret{{Name: "bob", Profile: "isolir", NeedsOriginalProfile: true}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	id := findByName(t, e, scope, r.ID, "bob")
	if err := repo.SetOriginalProfile(e.ctx, scope, id, "paket-10m"); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.FindByID(e.ctx, scope, id)
	if got.OriginalProfile != "paket-10m" || got.NeedsOriginalProfile {
		t.Fatalf("got %+v", got)
	}
	if err := repo.SetOriginalProfile(e.ctx, scope, "00000000-0000-0000-0000-00000000dead", "x"); !errors.Is(err, mikrotik.ErrSecretNotFound) {
		t.Fatalf("err = %v", err)
	}
}
