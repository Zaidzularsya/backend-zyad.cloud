package service

import (
	"context"
	"errors"
	"testing"

	"zyad.cloud/internal/modules/mikrotik"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

func TestSetupLockHeldRefusesApplyAndCleanupWithoutTouchingAnything(t *testing.T) {
	f := newSetupFixture(t)
	applyReady(t, f)
	plan := f.preview(t, validSetup())

	f.locker.held = true
	writes, marks := f.client.WriteCalls(), len(f.repo.setups)
	findsAtLock := -1
	f.locker.onLock = func() { findsAtLock = f.repo.finds }
	findsBefore := f.repo.finds

	if _, err := f.svc.Apply(context.Background(), testScope(t), f.routerID, validSetup(), plan.Hash, testActor); !errors.Is(err, mikrotik.ErrSetupInProgress) {
		t.Fatalf("apply err = %v, want ErrSetupInProgress", err)
	}
	if findsAtLock != findsBefore {
		t.Fatal("apply read router state before taking the lock")
	}
	if _, err := f.svc.Cleanup(context.Background(), testScope(t), f.routerID, testActor); !errors.Is(err, mikrotik.ErrSetupInProgress) {
		t.Fatalf("cleanup err = %v, want ErrSetupInProgress", err)
	}
	if f.repo.finds != findsBefore {
		t.Fatalf("router state was read %d times while the lock was held", f.repo.finds-findsBefore)
	}
	if f.client.WriteCalls() != writes || len(f.repo.setups) != marks {
		t.Fatal("held lock: router writes or MarkSetup happened")
	}
	if !f.repo.routers[f.routerID].IsolirReady {
		t.Fatal("held lock must leave isolir_ready untouched")
	}
	for _, k := range f.locker.keys {
		if k != "mikrotik.setup:"+testScope(t).OrganizationID()+":"+f.routerID {
			t.Fatalf("lock key = %q", k)
		}
	}
}

func TestSetupLockReleasedOnEveryPath(t *testing.T) {
	f := newSetupFixture(t)
	ctx, scope := context.Background(), testScope(t)
	check := func(label string) {
		t.Helper()
		if f.locker.acquired != f.locker.released {
			t.Fatalf("%s: acquired %d, released %d", label, f.locker.acquired, f.locker.released)
		}
	}

	// Success.
	applyReady(t, f)
	check("apply success")
	if _, err := f.svc.Cleanup(ctx, scope, f.routerID, testActor); err != nil {
		t.Fatal(err)
	}
	check("cleanup success")

	// Early return: stale hash (nothing written).
	if _, err := f.svc.Apply(ctx, scope, f.routerID, validSetup(), "stale", testActor); !errors.Is(err, ErrPlanChanged) {
		t.Fatalf("err = %v", err)
	}
	check("stale hash")

	// Error after writes started.
	f.client.WriteHook = func(string, string, pmk.Row) error { return errors.New("failure") }
	plan := f.preview(t, validSetup())
	if _, err := f.svc.Apply(ctx, scope, f.routerID, validSetup(), plan.Hash, testActor); err == nil {
		t.Fatal("expected failure")
	}
	check("apply failure")
	f.client.WriteHook = nil

	// Mark failure before cleanup (fail closed).
	applyReady(t, f)
	f.repo.markErr = errors.New("db down")
	if _, err := f.svc.Cleanup(ctx, scope, f.routerID, testActor); !errors.Is(err, mikrotik.ErrSetupMarkFailed) {
		t.Fatalf("err = %v", err)
	}
	check("cleanup mark failure")
	f.repo.markErr = nil

	// Unknown router: no lock is taken at all for a malformed id.
	before := f.locker.acquired
	if _, err := f.svc.Cleanup(ctx, scope, "not-a-uuid", testActor); !errors.Is(err, mikrotik.ErrRouterNotFound) {
		t.Fatalf("err = %v", err)
	}
	if f.locker.acquired != before {
		t.Fatal("malformed id must not take a lock")
	}
	check("not found")

	if f.locker.acquired == 0 {
		t.Fatal("lock never taken")
	}
}

func TestSetupLockErrorIsNotWritten(t *testing.T) {
	f := newSetupFixture(t)
	f.locker.err = errors.New("pool exhausted")
	writes := f.client.WriteCalls()
	_, err := f.svc.Cleanup(context.Background(), testScope(t), f.routerID, testActor)
	if err == nil || errors.Is(err, mikrotik.ErrSetupInProgress) {
		t.Fatalf("err = %v", err)
	}
	if f.client.WriteCalls() != writes || len(f.repo.setups) != 0 {
		t.Fatal("lock error must not write")
	}
}

func TestPreviewDoesNotLock(t *testing.T) {
	f := newSetupFixture(t)
	f.locker.held = true
	f.preview(t, validSetup())
	if len(f.locker.keys) != 0 {
		t.Fatal("preview must not take the setup lock")
	}
}

func TestSetupConcurrencyCapRefusesWithoutLockingAndReleasesSlot(t *testing.T) {
	f := newSetupFixture(t)
	for i := 0; i < cap(f.svc.slots); i++ {
		f.svc.slots <- struct{}{}
	}
	if _, err := f.svc.Cleanup(context.Background(), testScope(t), f.routerID, testActor); !errors.Is(err, mikrotik.ErrSetupInProgress) {
		t.Fatalf("cleanup err = %v, want ErrSetupInProgress", err)
	}
	if f.locker.acquired != 0 {
		t.Fatal("lock taken although all slots were busy")
	}
	for i := 0; i < cap(f.svc.slots); i++ {
		<-f.svc.slots
	}
	if _, err := f.svc.Cleanup(context.Background(), testScope(t), f.routerID, testActor); err != nil {
		t.Fatalf("cleanup after slots freed: %v", err)
	}
	if len(f.svc.slots) != 0 {
		t.Fatalf("slot leaked: %d in use", len(f.svc.slots))
	}
}
