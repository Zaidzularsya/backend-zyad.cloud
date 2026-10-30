package service

import (
	"context"
	"errors"
	"testing"

	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

// restFixture is a setup fixture whose router was detected on REST.
func restFixture(t *testing.T, allowREST bool) *setupFixture {
	t.Helper()
	f := newSetupFixture(t)
	r := f.repo.routers[f.routerID]
	r.APIType = domain.APITypeREST
	f.repo.routers[f.routerID] = r
	f.svc.opts.AllowREST = allowREST
	return f
}

func TestSetupOverRESTRefusedWhenFlagOff(t *testing.T) {
	f := restFixture(t, false)
	ctx, scope := context.Background(), testScope(t)

	// Preview stays allowed.
	plan := f.preview(t, validSetup())
	writes, marks := f.client.WriteCalls(), len(f.repo.setups)

	if _, err := f.svc.Apply(ctx, scope, f.routerID, validSetup(), plan.Hash, testActor); !errors.Is(err, mikrotik.ErrSetupRESTUnverified) {
		t.Fatalf("apply err = %v, want ErrSetupRESTUnverified", err)
	}
	if _, err := f.svc.Cleanup(ctx, scope, f.routerID, testActor); !errors.Is(err, mikrotik.ErrSetupRESTUnverified) {
		t.Fatalf("cleanup err = %v, want ErrSetupRESTUnverified", err)
	}
	if f.client.WriteCalls() != writes || len(f.repo.setups) != marks {
		t.Fatalf("REST gate: writes %d->%d, MarkSetup %d->%d; want none", writes, f.client.WriteCalls(), marks, len(f.repo.setups))
	}
	if f.locker.acquired != f.locker.released {
		t.Fatal("lock not released after refusal")
	}
}

func TestSetupOverRESTProceedsWhenFlagOn(t *testing.T) {
	f := restFixture(t, true)
	applyReady(t, f)
	if !f.repo.routers[f.routerID].IsolirReady {
		t.Fatal("apply over REST with the flag on must mark ready")
	}
	if _, err := f.svc.Cleanup(context.Background(), testScope(t), f.routerID, testActor); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

func TestBinaryRoutersUnaffectedByRESTFlag(t *testing.T) {
	f := newSetupFixture(t) // binary, flag off
	if f.repo.routers[f.routerID].APIType != domain.APITypeBinary || f.svc.opts.AllowREST {
		t.Fatal("precondition: binary router, flag off")
	}
	applyReady(t, f)
	if _, err := f.svc.Cleanup(context.Background(), testScope(t), f.routerID, testActor); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}
