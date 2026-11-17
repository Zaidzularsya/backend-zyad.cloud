//go:build integration

package repository_test

import (
	"testing"

	"zyad.cloud/internal/modules/mikrotik/repository"
)

func TestAccountIDsByName_CaseInsensitiveTrim(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	id := seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "Budi@ISP", Profile: "paket"})
	acct := newAccount(t, e, scope)
	if err := secretRepo(e).Pair(e.ctx, scope, id, acct, ""); err != nil {
		t.Fatal(err)
	}
	got, err := secretRepo(e).AccountIDsByName(e.ctx, scope, "  budi@isp ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != acct {
		t.Fatalf("got %v want [%s]", got, acct)
	}
}

func TestAccountIDsByName_DistinctAcrossRouters(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r1, r2 := e.create(t, scope, "R1"), e.create(t, scope, "R2")
	acct := newAccount(t, e, scope)
	for _, r := range []string{r1.ID, r2.ID} {
		id := seedSecret(t, e, scope, r, repository.ImportedSecret{Name: "dup", Profile: "paket"})
		if err := secretRepo(e).Pair(e.ctx, scope, id, acct, ""); err != nil {
			t.Fatal(err)
		}
	}
	got, err := secretRepo(e).AccountIDsByName(e.ctx, scope, "DUP")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != acct {
		t.Fatalf("got %v want one id %s", got, acct)
	}
}

func TestAccountIDsByName_TenantIsolation(t *testing.T) {
	e := setupRouter(t)
	a, b := e.tenants.A.Scope, e.tenants.B.Scope
	rb := e.create(t, b, "RB")
	idb := seedSecret(t, e, b, rb.ID, repository.ImportedSecret{Name: "shared", Profile: "paket"})
	acctB := newAccount(t, e, b)
	if err := secretRepo(e).Pair(e.ctx, b, idb, acctB, ""); err != nil {
		t.Fatal(err)
	}
	got, err := secretRepo(e).AccountIDsByName(e.ctx, a, "shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("tenant A saw %v", got)
	}
	if got, _ = secretRepo(e).AccountIDsByName(e.ctx, b, "shared"); len(got) != 1 {
		t.Fatalf("tenant B got %v", got)
	}
}

func TestAccountIDsByName_UnpairedExcluded(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	seedSecret(t, e, scope, r.ID, repository.ImportedSecret{Name: "loner", Profile: "paket"})
	got, err := secretRepo(e).AccountIDsByName(e.ctx, scope, "loner")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}
