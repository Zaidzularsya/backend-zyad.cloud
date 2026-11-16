//go:build integration

package repository_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/mikrotik/repository"
)

func TestSecretOnboarding_SaveGetAndScope(t *testing.T) {
	e := setupRouter(t)
	ctx, tenants := e.ctx, e.tenants
	router := e.create(t, tenants.A.Scope, "r-onboard")
	secretID := insertSecret(t, e, tenants.A.Scope, router.ID, "budi@pppoe", newAccount(t, e, tenants.A.Scope))
	repo := repository.NewSecretOnboardingRepository(e.db)

	if _, found, err := repo.Get(ctx, tenants.A.Scope, secretID); err != nil || found {
		t.Fatalf("empty Get: found=%v err=%v", found, err)
	}
	deal := "11111111-2222-3333-4444-555555555555"
	if err := repo.Save(ctx, tenants.A.Scope, repository.SecretOnboarding{SecretID: secretID, Stage: "deal", DealID: deal, LastError: "kuota"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, tenants.A.Scope, repository.SecretOnboarding{SecretID: secretID, Stage: "quotation", DealID: deal}, ""); err != nil {
		t.Fatal(err)
	}
	got, found, err := repo.Get(ctx, tenants.A.Scope, secretID)
	if err != nil || !found || got.Stage != "quotation" || got.DealID != deal || got.LastError != "" || got.ContactID != "" {
		t.Fatalf("Get = %+v found=%v err=%v", got, found, err)
	}
	list, err := repo.ListBySecrets(ctx, tenants.A.Scope, []string{secretID})
	if err != nil || list[secretID].Stage != "quotation" {
		t.Fatalf("ListBySecrets = %+v err=%v", list, err)
	}
	if _, found, _ := repo.Get(ctx, tenants.B.Scope, secretID); found {
		t.Fatal("tenant B must not see tenant A onboarding")
	}
	if err := repo.Save(ctx, tenants.A.Scope, repository.SecretOnboarding{SecretID: secretID, Stage: "bogus"}, ""); err == nil {
		t.Fatal("unknown stage must be rejected by the CHECK")
	}
}

func TestSecretCountByDisabled(t *testing.T) {
	e := setupRouter(t)
	a := e.tenants.A
	router := e.create(t, a.Scope, "r-count")
	acct := newAccount(t, e, a.Scope)
	on := insertSecret(t, e, a.Scope, router.ID, "on", acct)
	off := insertSecret(t, e, a.Scope, router.ID, "off", acct)
	gone := insertSecret(t, e, a.Scope, router.ID, "gone", acct)
	secretScopedTx(t, e, a.Scope, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_ppp_secrets SET disabled = true WHERE id = $1`, off); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_ppp_secrets SET present = false WHERE id = $1`, gone); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(e.ctx); err != nil {
			t.Fatal(err)
		}
	})
	_ = on
	repo := repository.NewSecretRepository(e.db)
	active, inactive, err := repo.CountByDisabled(e.ctx, a.Scope, "")
	if err != nil || active != 1 || inactive != 1 {
		t.Fatalf("all routers: %d/%d err=%v", active, inactive, err)
	}
	if active, inactive, _ := repo.CountByDisabled(e.ctx, a.Scope, router.ID); active != 1 || inactive != 1 {
		t.Fatalf("router filter: %d/%d", active, inactive)
	}
	if active, inactive, _ := repo.CountByDisabled(e.ctx, e.tenants.B.Scope, ""); active != 0 || inactive != 0 {
		t.Fatalf("tenant B sees %d/%d", active, inactive)
	}
}
