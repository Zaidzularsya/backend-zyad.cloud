//go:build integration

package repository_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

func policyRepo(e routerEnv) repository.PolicyRepository { return repository.NewPolicyRepository(e.db) }

func TestPolicyRepo_UpsertGetAndDefault(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	acct := newAccount(t, e, scope)

	if _, found, err := policyRepo(e).GetCustomerPolicy(e.ctx, scope, acct); err != nil || found {
		t.Fatalf("before: found=%v err=%v", found, err)
	}
	p, err := policyRepo(e).UpsertCustomerPolicy(e.ctx, scope, acct, true, "VIP", nil, "")
	if err != nil || !p.Exempt || p.Reason != "VIP" || p.SetBy != nil {
		t.Fatalf("upsert = %+v, %v", p, err)
	}
	p, err = policyRepo(e).UpsertCustomerPolicy(e.ctx, scope, acct, false, "dicabut", nil, "")
	if err != nil || p.Exempt || p.Reason != "dicabut" {
		t.Fatalf("second upsert = %+v, %v", p, err)
	}
	got, found, err := policyRepo(e).GetCustomerPolicy(e.ctx, scope, acct)
	if err != nil || !found || got.Exempt || got.Reason != "dicabut" {
		t.Fatalf("get = %+v found=%v err=%v", got, found, err)
	}
	var n int
	secretScopedTx(t, e, scope, func(tx pgx.Tx) {
		_ = tx.QueryRow(e.ctx, `SELECT count(*) FROM mikrotik_customer_policies`).Scan(&n)
	})
	if n != 1 {
		t.Fatalf("rows = %d", n)
	}
}

func TestPolicyRepo_ForeignAccountAndRLS(t *testing.T) {
	e := setupRouter(t)
	a, b := e.tenants.A.Scope, e.tenants.B.Scope
	acctA := newAccount(t, e, a)

	if _, err := policyRepo(e).UpsertCustomerPolicy(e.ctx, b, acctA, true, "x", nil, ""); !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("foreign account err = %v", err)
	}
	if _, err := policyRepo(e).UpsertCustomerPolicy(e.ctx, a, acctA, true, "x", nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, found, err := policyRepo(e).GetCustomerPolicy(e.ctx, b, acctA); err != nil || found {
		t.Fatalf("cross-org read found=%v err=%v", found, err)
	}
}

func TestPolicyRepo_ExemptedUntilRoundTrip(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	acct := newAccount(t, e, scope)
	until := time.Date(2030, 12, 31, 0, 0, 0, 0, time.UTC)

	p, err := policyRepo(e).UpsertCustomerPolicy(e.ctx, scope, acct, true, "VIP", &until, "")
	if err != nil || p.ExemptedUntil == nil || !p.ExemptedUntil.Equal(until) {
		t.Fatalf("upsert = %+v, %v", p, err)
	}
	got, found, err := policyRepo(e).GetCustomerPolicy(e.ctx, scope, acct)
	if err != nil || !found || got.ExemptedUntil == nil || got.ExemptedUntil.Format("2006-01-02") != "2030-12-31" {
		t.Fatalf("get = %+v found=%v err=%v", got, found, err)
	}
	p, err = policyRepo(e).UpsertCustomerPolicy(e.ctx, scope, acct, true, "VIP", nil, "")
	if err != nil || p.ExemptedUntil != nil {
		t.Fatalf("clear = %+v, %v", p, err)
	}
}
