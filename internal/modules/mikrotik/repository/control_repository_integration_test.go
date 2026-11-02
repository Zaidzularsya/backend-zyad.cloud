//go:build integration

package repository_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

func controlRepo(e routerEnv) repository.ControlRepository {
	return repository.NewControlRepository(e.db)
}

func TestControlRepo_DefaultThenSet(t *testing.T) {
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	got, err := controlRepo(e).Get(e.ctx, scope)
	if err != nil || got.Mode != domain.ModeSimulation || got.GraceDays != 0 || got.PilotVerifiedAt != nil || got.OrganizationID != scope.OrganizationID() {
		t.Fatalf("default = %+v, %v", got, err)
	}
	var n int
	secretScopedTx(t, e, scope, func(tx pgx.Tx) {
		_ = tx.QueryRow(e.ctx, `SELECT count(*) FROM mikrotik_org_settings`).Scan(&n)
	})
	if n != 0 {
		t.Fatalf("Get must not insert, rows=%d", n)
	}
	s, err := controlRepo(e).SetGraceDays(e.ctx, scope, 7, "")
	if err != nil || s.GraceDays != 7 || s.Mode != domain.ModeSimulation {
		t.Fatalf("SetGraceDays = %+v, %v", s, err)
	}
	s, err = controlRepo(e).SetMode(e.ctx, scope, domain.ModePilot, "")
	if err != nil || s.Mode != domain.ModePilot || s.GraceDays != 7 {
		t.Fatalf("SetMode = %+v, %v", s, err)
	}
	if _, err := controlRepo(e).SetGraceDays(e.ctx, scope, 61, ""); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("grace 61 err = %v", err)
	}
	if _, err := controlRepo(e).SetGraceDays(e.ctx, scope, -1, ""); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("grace -1 err = %v", err)
	}
	if _, err := controlRepo(e).SetMode(e.ctx, scope, "bogus", ""); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("bogus mode err = %v", err)
	}
	if got, _ := controlRepo(e).Get(e.ctx, e.tenants.B.Scope); got.Mode != domain.ModeSimulation || got.GraceDays != 0 {
		t.Fatalf("org B must be untouched: %+v", got)
	}
}

func TestControlRepo_ActivateWithExemptions_Atomic(t *testing.T) {
	e := setupRouter(t)
	a, b := e.tenants.A.Scope, e.tenants.B.Scope
	acctA := newAccount(t, e, a)
	acctB := newAccount(t, e, b)
	until := time.Date(2030, 1, 31, 0, 0, 0, 0, time.UTC)

	_, err := controlRepo(e).ActivateWithExemptions(e.ctx, a, domain.ModeActive, []repository.Exemption{
		{AccountID: acctA, Until: &until, Reason: "VIP"},
		{AccountID: acctB, Reason: "foreign"},
	}, "")
	if !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("err = %v, want ErrAccountNotFound", err)
	}
	got, _ := controlRepo(e).Get(e.ctx, a)
	if got.Mode != domain.ModeSimulation {
		t.Fatalf("mode changed to %q despite failure", got.Mode)
	}
	var n int
	secretScopedTx(t, e, a, func(tx pgx.Tx) {
		_ = tx.QueryRow(e.ctx, `SELECT count(*) FROM mikrotik_customer_policies`).Scan(&n)
	})
	if n != 0 {
		t.Fatalf("policy rows = %d, want 0", n)
	}

	s, err := controlRepo(e).ActivateWithExemptions(e.ctx, a, domain.ModeActive, []repository.Exemption{
		{AccountID: acctA, Until: &until, Reason: "VIP"},
	}, "")
	if err != nil || s.Mode != domain.ModeActive {
		t.Fatalf("activate = %+v, %v", s, err)
	}
	p, found, err := policyRepo(e).GetCustomerPolicy(e.ctx, a, acctA)
	if err != nil || !found || !p.Exempt || p.Reason != "VIP" || p.ExemptedUntil == nil || !p.ExemptedUntil.Equal(until) {
		t.Fatalf("policy = %+v found=%v err=%v", p, found, err)
	}
}

func TestControlRepo_PilotAddIdempotentRemove(t *testing.T) {
	e := setupRouter(t)
	a, b := e.tenants.A.Scope, e.tenants.B.Scope
	acct := newAccount(t, e, a)
	acct2 := newAccount(t, e, a)
	r := controlRepo(e)

	if err := r.AddPilot(e.ctx, a, acct, ""); err != nil {
		t.Fatal(err)
	}
	if err := r.AddPilot(e.ctx, a, acct, ""); err != nil {
		t.Fatalf("second add: %v", err)
	}
	if err := r.AddPilot(e.ctx, a, acct2, ""); err != nil {
		t.Fatal(err)
	}
	list, err := r.ListPilot(e.ctx, a)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := r.AddPilot(e.ctx, b, acct, ""); !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("foreign add err = %v", err)
	}
	if list, _ := r.ListPilot(e.ctx, b); len(list) != 0 {
		t.Fatalf("org B list = %+v", list)
	}
	if err := r.RemovePilot(e.ctx, a, acct); err != nil {
		t.Fatal(err)
	}
	if err := r.RemovePilot(e.ctx, a, acct); err != nil {
		t.Fatalf("second remove: %v", err)
	}
	if list, _ := r.ListPilot(e.ctx, a); len(list) != 1 || list[0].AccountID != acct2 {
		t.Fatalf("after remove = %+v", list)
	}
}

func TestControlRepo_PilotStateTestActiveWindow(t *testing.T) {
	e := setupRouter(t)
	a := e.tenants.A.Scope
	acct := newAccount(t, e, a)
	r := controlRepo(e)

	if isPilot, active, err := r.PilotState(e.ctx, a, acct); err != nil || isPilot || active {
		t.Fatalf("not pilot: %v %v %v", isPilot, active, err)
	}
	if err := r.StartPilotTest(e.ctx, a, acct, time.Now().Add(time.Hour)); !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("start on non-pilot err = %v", err)
	}
	if err := r.StopPilotTest(e.ctx, a, acct); !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("stop on non-pilot err = %v", err)
	}
	if err := r.AddPilot(e.ctx, a, acct, ""); err != nil {
		t.Fatal(err)
	}
	if isPilot, active, err := r.PilotState(e.ctx, a, acct); err != nil || !isPilot || active {
		t.Fatalf("pilot idle: %v %v %v", isPilot, active, err)
	}
	if err := r.StartPilotTest(e.ctx, a, acct, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, active, _ := r.PilotState(e.ctx, a, acct); !active {
		t.Fatal("test window must be active")
	}
	if err := r.StopPilotTest(e.ctx, a, acct); err != nil {
		t.Fatal(err)
	}
	if _, active, _ := r.PilotState(e.ctx, a, acct); active {
		t.Fatal("test window must be closed after stop")
	}
	if err := r.StartPilotTest(e.ctx, a, acct, time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, active, _ := r.PilotState(e.ctx, a, acct); active {
		t.Fatal("expired window must be inactive")
	}
	list, _ := r.ListPilot(e.ctx, a)
	if len(list) != 1 || list[0].TestStartedAt == nil || list[0].TestUntil == nil {
		t.Fatalf("list = %+v", list)
	}
}

func TestControlRepo_MarkPilotRoundTrip(t *testing.T) {
	e := setupRouter(t)
	a := e.tenants.A.Scope
	acct := newAccount(t, e, a)
	r := controlRepo(e)
	router := e.create(t, a, "R1")
	secret := seedSecret(t, e, a, router.ID, repository.ImportedSecret{Name: "u1", Profile: "default"})
	secretScopedTx(t, e, a, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_ppp_secrets SET account_id = $2 WHERE id = $1`, secret, acct); err != nil {
			t.Fatal(err)
		}
		_ = tx.Commit(e.ctx)
	})
	if err := r.AddPilot(e.ctx, a, acct, ""); err != nil {
		t.Fatal(err)
	}
	if v, err := r.MarkPilotRoundTrip(e.ctx, a, acct); err != nil || v {
		t.Fatalf("no test started: %v %v", v, err)
	}
	if err := r.StartPilotTest(e.ctx, a, acct, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if v, err := r.MarkPilotRoundTrip(e.ctx, a, acct); err != nil || v {
		t.Fatalf("no done task: %v %v", v, err)
	}
	secretScopedTx(t, e, a, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, `INSERT INTO mikrotik_isolation_tasks (organization_id, secret_id, action, status, finished_at)
			VALUES ($1, $2, 'isolate', 'done', now())`, a.OrganizationID(), secret); err != nil {
			t.Fatal(err)
		}
		_ = tx.Commit(e.ctx)
	})
	if v, err := r.MarkPilotRoundTrip(e.ctx, a, acct); err != nil || !v {
		t.Fatalf("done task: %v %v", v, err)
	}
	if s, _ := r.Get(e.ctx, a); s.PilotVerifiedAt == nil {
		t.Fatal("pilot_verified_at must be set")
	}
}
