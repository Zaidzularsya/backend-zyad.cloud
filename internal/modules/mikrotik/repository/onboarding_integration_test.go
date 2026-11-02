//go:build integration

package repository_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

// cleanPolicyRows removes the policy/trigger rows a test created so sibling
// tests that list all rows of the organization are not affected.
func cleanPolicyRows(t *testing.T, e routerEnv) {
	t.Helper()
	t.Cleanup(func() {
		for _, scope := range []coretenant.Scope{e.tenants.A.Scope, e.tenants.B.Scope} {
			secretScopedTx(t, e, scope, func(tx pgx.Tx) {
				_, _ = tx.Exec(e.ctx, `DELETE FROM mikrotik_customer_policies`)
				_, _ = tx.Exec(e.ctx, `DELETE FROM mikrotik_product_triggers`)
				_ = tx.Commit(e.ctx)
			})
		}
	})
}

func onboardingRepo(e routerEnv) *repository.OnboardingRepository {
	return repository.NewOnboardingRepository(e.db)
}

// Leftover product triggers of other tests are not cleaned up, so product
// counts are asserted as deltas against a baseline.
func TestOnboardingRepo_Counts(t *testing.T) {
	e := setupRouter(t)
	cleanPolicyRows(t, e)
	a, b := e.tenants.A.Scope, e.tenants.B.Scope
	repo := onboardingRepo(e)

	base, err := repo.Counts(e.ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if base.Routers != 0 || base.SecretsTotal != 0 {
		t.Fatalf("setupRouter should leave no routers/secrets: %+v", base)
	}

	r1 := e.create(t, a, "R-on-1")
	r2 := e.create(t, a, "R-on-2")
	rb := e.create(t, b, "R-on-b")
	// r1 ready, r2 not ready; deleted router must not count at all.
	if err := e.repo.MarkSetup(e.ctx, a, r1.ID, true, "isolir", "http://x"); err != nil {
		t.Fatal(err)
	}
	gone := e.create(t, a, "R-on-gone")
	if err := e.repo.MarkSetup(e.ctx, a, gone.ID, true, "isolir", "http://x"); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.SoftDelete(e.ctx, a, gone.ID, ""); err != nil {
		t.Fatal(err)
	}
	_ = r2

	paired := seedSecret(t, e, a, r1.ID, repository.ImportedSecret{Name: "paired", Profile: "p"})
	seedSecret(t, e, a, r1.ID, repository.ImportedSecret{Name: "unpaired", Profile: "p"})
	seedSecret(t, e, a, r1.ID, repository.ImportedSecret{Name: "needs", Profile: "isolir", NeedsOriginalProfile: true})
	missing := seedSecret(t, e, a, r1.ID, repository.ImportedSecret{Name: "missing", Profile: "p", NeedsOriginalProfile: true})
	seedSecret(t, e, b, rb.ID, repository.ImportedSecret{Name: "other-org", Profile: "p"})
	if err := secretRepo(e).Pair(e.ctx, a, paired, newAccount(t, e, a), ""); err != nil {
		t.Fatal(err)
	}
	// "missing" disappeared from the router: excluded from every secret count.
	if err := secretRepo(e).MarkAbsent(e.ctx, a, missing); err != nil {
		t.Fatal(err)
	}

	pol := policyRepo(e)
	pidOn := fmt.Sprintf("00000000-0000-0000-0000-%012d", time.Now().UnixNano()%1_000_000_000_000)
	pidOff := fmt.Sprintf("00000000-0000-0000-0001-%012d", time.Now().UnixNano()%1_000_000_000_000)
	day := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	if _, err := pol.UpsertProductTrigger(e.ctx, a, pidOn, true, day, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := pol.UpsertProductTrigger(e.ctx, a, pidOff, false, day, ""); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Counts(e.ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	want := repository.OnboardingCounts{
		Routers: 2, RoutersReady: 1,
		SecretsTotal: 3, SecretsPaired: 1, NeedsProfile: 1,
		ProductsMarked: base.ProductsMarked + 1,
	}
	if got != want {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}

	// RLS: org B sees only its own rows.
	gb, err := repo.Counts(e.ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if gb.Routers != 1 || gb.SecretsTotal != 1 || gb.SecretsPaired != 0 {
		t.Fatalf("org B counts = %+v", gb)
	}
}

func TestOnboardingRepo_AccountStatuses(t *testing.T) {
	e := setupRouter(t)
	cleanPolicyRows(t, e)
	a, b := e.tenants.A.Scope, e.tenants.B.Scope
	repo := onboardingRepo(e)
	r := e.create(t, a, "R-st")
	rb := e.create(t, b, "R-st-b")

	isolatedAcct := newAccount(t, e, a)
	normalAcct := newAccount(t, e, a)
	exemptOnly := newAccount(t, e, a)
	noneAcct := newAccount(t, e, a)
	foreignAcct := newAccount(t, e, b)

	pairTo := func(scope coretenant.Scope, routerID, name, acct string, isolated bool) {
		sid := seedSecret(t, e, scope, routerID, repository.ImportedSecret{Name: name, Profile: "p"})
		secretScopedTx(t, e, scope, func(tx pgx.Tx) {
			state := "normal"
			if isolated {
				state = "isolated"
			}
			if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_ppp_secrets SET account_id = $1, original_profile = 'paket', isolation_state = $2 WHERE id = $3`, acct, state, sid); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(e.ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
	// isolatedAcct: one normal + one isolated secret -> isolated, 2 secrets.
	pairTo(a, r.ID, "iso-1", isolatedAcct, false)
	pairTo(a, r.ID, "iso-2", isolatedAcct, true)
	pairTo(a, r.ID, "norm-1", normalAcct, false)
	pairTo(b, rb.ID, "foreign-1", foreignAcct, true)
	if _, err := policyRepo(e).UpsertCustomerPolicy(e.ctx, a, normalAcct, true, "VIP", nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := policyRepo(e).UpsertCustomerPolicy(e.ctx, a, exemptOnly, true, "VIP", nil, ""); err != nil {
		t.Fatal(err)
	}

	got, err := repo.AccountStatuses(e.ctx, a, []string{isolatedAcct, normalAcct, exemptOnly, noneAcct, foreignAcct})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]repository.AccountStatus{
		isolatedAcct: {Secrets: 2, Isolated: true},
		normalAcct:   {Secrets: 1, Exempt: true},
		exemptOnly:   {Exempt: true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s = %+v, want %+v", id, got[id], w)
		}
	}
	if _, ok := got[foreignAcct]; ok {
		t.Fatal("foreign organization account leaked")
	}

	if empty, err := repo.AccountStatuses(e.ctx, a, nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty input: %v %v", empty, err)
	}
}
