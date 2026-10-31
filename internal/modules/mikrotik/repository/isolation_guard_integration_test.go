//go:build integration

package repository_test

import (
	"errors"
	"testing"
	"time"

	"zyad.cloud/internal/modules/mikrotik"
)

func TestRouterSoftDelete_RefusedWithIsolatedSecret(t *testing.T) {
	f := newIsoFixture(t)
	if ok, err := secretRepo(f.e).MarkIsolated(f.e.ctx, f.scope, f.secretID, ""); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := f.e.repo.SoftDelete(f.e.ctx, f.scope, f.routerID, ""); !errors.Is(err, mikrotik.ErrRouterHasIsolatedSecrets) {
		t.Fatalf("isolated secret: err = %v", err)
	}
	if _, err := f.e.repo.FindByID(f.e.ctx, f.scope, f.routerID); err != nil {
		t.Fatalf("router must still exist: %v", err)
	}
	if ok, err := secretRepo(f.e).MarkNormal(f.e.ctx, f.scope, f.secretID); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := f.e.repo.SoftDelete(f.e.ctx, f.scope, f.routerID, ""); err != nil {
		t.Fatalf("after restore: %v", err)
	}
}

func TestRouterSoftDelete_RefusedWithActiveTask(t *testing.T) {
	f := newIsoFixture(t)
	repo := taskRepo(f.e)
	if ok, err := repo.EnqueueIsolate(f.e.ctx, f.scope, f.secretID, "x"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := f.e.repo.SoftDelete(f.e.ctx, f.scope, f.routerID, ""); !errors.Is(err, mikrotik.ErrRouterHasIsolatedSecrets) {
		t.Fatalf("pending task: err = %v", err)
	}
	claimed, err := repo.ClaimBatch(f.e.ctx, f.scope, 1, time.Now().UTC().Add(time.Second), time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatal(err, claimed)
	}
	if err := f.e.repo.SoftDelete(f.e.ctx, f.scope, f.routerID, ""); !errors.Is(err, mikrotik.ErrRouterHasIsolatedSecrets) {
		t.Fatalf("running task: err = %v", err)
	}
	if err := repo.Fail(f.e.ctx, f.scope, claimed[0].ID, "auth_failed", "x"); err != nil {
		t.Fatal(err)
	}
	if err := f.e.repo.SoftDelete(f.e.ctx, f.scope, f.routerID, ""); err != nil {
		t.Fatalf("terminal task: %v", err)
	}
}

func TestRouterSoftDelete_UnknownStillNotFound(t *testing.T) {
	f := newIsoFixture(t)
	if err := f.e.repo.SoftDelete(f.e.ctx, f.scope, "00000000-0000-0000-0000-000000000001", ""); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	// Another organization cannot see (or be blocked by) this router.
	if err := f.e.repo.SoftDelete(f.e.ctx, f.e.tenants.B.Scope, f.routerID, ""); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("cross-org: %v", err)
	}
}

func TestUnpair_RefusedWhileTaskPendingOrRunning(t *testing.T) {
	f := newIsoFixture(t)
	repo, secrets := taskRepo(f.e), secretRepo(f.e)
	if ok, err := repo.EnqueueIsolate(f.e.ctx, f.scope, f.secretID, "x"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := secrets.Unpair(f.e.ctx, f.scope, f.secretID); !errors.Is(err, mikrotik.ErrSecretHasActiveTask) {
		t.Fatalf("pending: err = %v", err)
	}
	claimed, err := repo.ClaimBatch(f.e.ctx, f.scope, 1, time.Now().UTC().Add(time.Second), time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatal(err, claimed)
	}
	// The race the guard closes: a running isolate must not lose its pairing.
	if err := secrets.Unpair(f.e.ctx, f.scope, f.secretID); !errors.Is(err, mikrotik.ErrSecretHasActiveTask) {
		t.Fatalf("running: err = %v", err)
	}
	s, err := secrets.FindByID(f.e.ctx, f.scope, f.secretID)
	if err != nil || s.AccountID == nil {
		t.Fatalf("pairing must be kept: %v %+v", err, s)
	}
	if err := repo.Complete(f.e.ctx, f.scope, claimed[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Unpair(f.e.ctx, f.scope, f.secretID); err != nil {
		t.Fatalf("terminal task: %v", err)
	}
}

func TestUnpair_IsolatedStillReportsIsolated(t *testing.T) {
	f := newIsoFixture(t)
	if ok, err := secretRepo(f.e).MarkIsolated(f.e.ctx, f.scope, f.secretID, ""); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := taskRepo(f.e).EnqueueRestore(f.e.ctx, f.scope, f.secretID, "x"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := secretRepo(f.e).Unpair(f.e.ctx, f.scope, f.secretID); !errors.Is(err, mikrotik.ErrSecretIsolated) {
		t.Fatalf("isolated with task: err = %v", err)
	}
}
