//go:build integration

package repository_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
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

func TestSecretRepo_ListSuggestionFilter(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R-sugfilter")
	other := e.create(t, scope, "R-sugfilter-other")
	phone := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "s-phone", Profile: "p"})
	amb := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "s-amb", Profile: "p"})
	none := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "s-none", Profile: "p"})
	paired := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "s-paired", Profile: "p"})
	foreign := seedSecret(t, e, scope, other.ID, repository.ImportedSecret{Name: "s-other", Profile: "p"})
	acct := newAccount(t, e, scope)
	sugRepo := repository.NewSuggestionRepository(e.db)
	if err := sugRepo.ReplaceForRouter(e.ctx, scope, r.ID, []repository.Suggestion{
		{SecretID: phone, AccountID: acct, Basis: "phone", Candidates: 1},
		{SecretID: amb, Basis: "ambiguous", Candidates: 2},
		{SecretID: paired, AccountID: acct, Basis: "name", Candidates: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sugRepo.ReplaceForRouter(e.ctx, scope, other.ID, []repository.Suggestion{{SecretID: foreign, AccountID: acct, Basis: "name", Candidates: 1}}); err != nil {
		t.Fatal(err)
	}
	// A hint on an already-paired secret is stale and must not surface.
	secretScopedTx(t, e, scope, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_ppp_secrets SET account_id = $1 WHERE id = $2`, acct, paired); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(e.ctx); err != nil {
			t.Fatal(err)
		}
	})

	repo := secretRepo(e)
	ids := func(f repository.SecretFilter) (map[string]domain.PPPSecret, int64) {
		f.RouterID = r.ID
		items, total, err := repo.List(e.ctx, scope, f, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]domain.PPPSecret{}
		for _, it := range items {
			m[it.ID] = it
		}
		return m, total
	}

	got, total := ids(repository.SecretFilter{Suggestion: repository.SuggestionFilterSuggested})
	if total != 1 || len(got) != 1 || got[phone].Suggestion == nil {
		t.Fatalf("suggested: total=%d got=%v", total, got)
	}

	// A suggested secret that vanished from the router is no longer actionable.
	gone := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "s-gone", Profile: "p"})
	if err := sugRepo.ReplaceForRouter(e.ctx, scope, r.ID, []repository.Suggestion{
		{SecretID: phone, AccountID: acct, Basis: "phone", Candidates: 1},
		{SecretID: amb, Basis: "ambiguous", Candidates: 2},
		{SecretID: paired, AccountID: acct, Basis: "name", Candidates: 1},
		{SecretID: gone, AccountID: acct, Basis: "phone", Candidates: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkAbsent(e.ctx, scope, gone); err != nil {
		t.Fatal(err)
	}
	got, total = ids(repository.SecretFilter{Suggestion: repository.SuggestionFilterSuggested})
	if total != 1 || len(got) != 1 || got[phone].Suggestion == nil || got[gone].ID != "" {
		t.Fatalf("suggested must exclude absent secrets: total=%d got=%v", total, got)
	}
	sg := got[phone].Suggestion
	if sg.AccountID != acct || sg.AccountName != "Cust" || sg.Basis != "phone" || sg.Candidates != 1 {
		t.Fatalf("suggestion = %+v", sg)
	}

	got, total = ids(repository.SecretFilter{Suggestion: repository.SuggestionFilterNeedsDecision})
	if total != 2 || len(got) != 2 || got[amb].Suggestion == nil || got[none].Suggestion != nil {
		t.Fatalf("needs_decision: total=%d got=%v", total, got)
	}
	if a := got[amb].Suggestion; a.Basis != "ambiguous" || a.AccountID != "" || a.Candidates != 2 {
		t.Fatalf("ambiguous = %+v", a)
	}

	// Composes with the other filters and keeps total consistent with the page.
	unpaired := false
	got, total = ids(repository.SecretFilter{Suggestion: repository.SuggestionFilterNeedsDecision, Paired: &unpaired, Search: "none"})
	if total != 1 || len(got) != 1 || got[none].ID == "" {
		t.Fatalf("composed: total=%d got=%v", total, got)
	}

	// Unfiltered listing: stale paired hint hidden, other router's secret absent.
	got, total = ids(repository.SecretFilter{})
	if total != 5 || got[paired].Suggestion != nil || got[phone].Suggestion == nil {
		t.Fatalf("unfiltered: total=%d got=%v", total, got)
	}

	// Another organization sees none of it.
	items, tot, err := repo.List(e.ctx, e.tenants.B.Scope, repository.SecretFilter{Suggestion: repository.SuggestionFilterSuggested}, 50, 0)
	if err != nil || tot != 0 || len(items) != 0 {
		t.Fatalf("org B: %v %d %d", err, tot, len(items))
	}
}

func TestSecretRepo_ListNeedsOriginalProfileAndAccountName(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R-needs")
	other := e.create(t, scope, "R-needs-other")
	needsU := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "n-unpaired", Profile: "iso", NeedsOriginalProfile: true})
	okU := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "o-unpaired", Profile: "p"})
	pairedLive := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "p-live", Profile: "p"})
	pairedDel := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "p-deleted", Profile: "p"})
	needsOther := seedSecret(t, e, scope, other.ID, repository.ImportedSecret{Name: "n-other", Profile: "iso", NeedsOriginalProfile: true})
	live := newAccount(t, e, scope)
	del := newAccount(t, e, scope)
	secretScopedTx(t, e, scope, func(tx pgx.Tx) {
		for _, q := range []struct{ sec, acc string }{{pairedLive, live}, {pairedDel, del}} {
			if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_ppp_secrets SET account_id = $1 WHERE id = $2`, q.acc, q.sec); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := tx.Exec(e.ctx, `UPDATE receivable_accounts SET deleted_at = now() WHERE id = $1`, del); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(e.ctx); err != nil {
			t.Fatal(err)
		}
	})

	repo := secretRepo(e)
	list := func(f repository.SecretFilter) (map[string]domain.PPPSecret, int64) {
		items, total, err := repo.List(e.ctx, scope, f, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]domain.PPPSecret{}
		for _, it := range items {
			m[it.ID] = it
		}
		return m, total
	}
	yes, no, unpaired := true, false, false

	got, total := list(repository.SecretFilter{NeedsOriginalProfile: &yes})
	if total != 2 || len(got) != 2 || got[needsU].ID == "" || got[needsOther].ID == "" {
		t.Fatalf("true: total=%d got=%v", total, got)
	}
	got, total = list(repository.SecretFilter{NeedsOriginalProfile: &yes, Paired: &unpaired, RouterID: r.ID})
	if total != 1 || len(got) != 1 || got[needsU].ID == "" {
		t.Fatalf("true+unpaired+router: total=%d got=%v", total, got)
	}
	got, total = list(repository.SecretFilter{NeedsOriginalProfile: &no, RouterID: r.ID})
	if total != 3 || len(got) != 3 || got[needsU].ID != "" {
		t.Fatalf("false+router: total=%d got=%v", total, got)
	}
	got, total = list(repository.SecretFilter{NeedsOriginalProfile: &no, Paired: &unpaired, RouterID: r.ID})
	if total != 1 || len(got) != 1 || got[okU].ID == "" {
		t.Fatalf("false+unpaired+router: total=%d got=%v", total, got)
	}

	got, total = list(repository.SecretFilter{RouterID: r.ID})
	if total != 4 {
		t.Fatalf("unfiltered total = %d", total)
	}
	if got[pairedLive].AccountName != "Cust" {
		t.Fatalf("paired account name = %q", got[pairedLive].AccountName)
	}
	if got[okU].AccountName != "" || got[needsU].AccountName != "" {
		t.Fatalf("unpaired must have empty name: %q %q", got[okU].AccountName, got[needsU].AccountName)
	}
	if got[pairedDel].AccountID == nil || got[pairedDel].AccountName != "" {
		t.Fatalf("soft-deleted account must give empty name: %+v", got[pairedDel])
	}

	// Org B sees nothing of org A, including account names.
	items, tot, err := repo.List(e.ctx, e.tenants.B.Scope, repository.SecretFilter{}, 50, 0)
	if err != nil || tot != 0 || len(items) != 0 {
		t.Fatalf("org B: %v %d %d", err, tot, len(items))
	}
}

// The composite FK (organization_id, account_id) normally makes a cross-org
// pairing impossible, so the a2 join's tenant predicate is only observable by
// building the row with the FK dropped. The test owns the table (non-superuser
// role), drops fk_mikrotik_ppp_secrets_account, commits the forged row, and a
// cleanup deletes it and re-adds the constraint (validated) afterwards.
// RLS on receivable_accounts would hide org A's account from org B on its own
// (the explicit join predicate is defense in depth), so RLS on that table is
// switched off for the duration to make the predicate itself observable.
func TestSecretRepo_ListAccountNameIsTenantScoped(t *testing.T) {
	e := setupRouter(t)
	a, b := e.tenants.A.Scope, e.tenants.B.Scope
	acctA := newAccount(t, e, a)
	rb := e.create(t, b, "R-forged")
	forged := seedSecret(t, e, b, rb.ID, repository.ImportedSecret{Name: "forged", Profile: "p"})

	const fk = "fk_mikrotik_ppp_secrets_account"
	if _, err := e.db.Exec(e.ctx, `ALTER TABLE mikrotik_ppp_secrets DROP CONSTRAINT `+fk); err != nil {
		t.Fatalf("drop fk: %v", err)
	}
	if _, err := e.db.Exec(e.ctx, `ALTER TABLE receivable_accounts NO FORCE ROW LEVEL SECURITY, DISABLE ROW LEVEL SECURITY`); err != nil {
		t.Fatalf("disable rls: %v", err)
	}
	t.Cleanup(func() {
		if _, err := e.db.Exec(e.ctx, `ALTER TABLE receivable_accounts ENABLE ROW LEVEL SECURITY, FORCE ROW LEVEL SECURITY`); err != nil {
			t.Errorf("restore rls: %v", err)
		}
		if _, err := e.db.Exec(e.ctx, `DELETE FROM mikrotik_ppp_secrets WHERE id = $1`, forged); err != nil {
			t.Errorf("cleanup delete: %v", err)
		}
		if _, err := e.db.Exec(e.ctx, `ALTER TABLE mikrotik_ppp_secrets ADD CONSTRAINT `+fk+
			` FOREIGN KEY (organization_id, account_id) REFERENCES receivable_accounts(organization_id, id)`); err != nil {
			t.Errorf("restore fk: %v", err)
		}
	})
	secretScopedTx(t, e, b, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_ppp_secrets SET account_id = $1 WHERE id = $2`, acctA, forged); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(e.ctx); err != nil {
			t.Fatal(err)
		}
	})

	items, total, err := secretRepo(e).List(e.ctx, b, repository.SecretFilter{RouterID: rb.ID}, 50, 0)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("org B list: err=%v total=%d n=%d", err, total, len(items))
	}
	if items[0].AccountID == nil || items[0].AccountName != "" {
		t.Fatalf("org A account name leaked into org B: %+v", items[0])
	}
}
