package service

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/mikrotik/domain"
)

func TestEvaluator_ApplyRestoreOnly_NeverEnqueuesIsolate(t *testing.T) {
	// Account with a trigger invoice: one normal secret (Apply would isolate)
	// and one isolated secret (already matches). Restore-only must enqueue nothing.
	e := newEvalEnv(t, ModeOn)
	e.src.invoices = oneInvoice
	e.sec.rows = []domain.PPPSecret{
		secretRow("s-normal", domain.IsolationNormal, true, false),
		secretRow("s-iso", domain.IsolationIsolated, true, false),
	}
	dec, err := e.ev.ApplyRestoreOnly(context.Background(), e.scope, "acc-1", "reconcile")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.tasks.isolate) != 0 || len(e.tasks.restore) != 0 {
		t.Fatalf("isolate=%v restore=%v, want none", e.tasks.isolate, e.tasks.restore)
	}
	for _, a := range dec.Secrets {
		if a.Action == actionIsolate {
			t.Fatalf("decision carries isolate action: %+v", dec.Secrets)
		}
	}

	// Paid (no trigger invoice): the isolated secret is restored.
	e.src.invoices = nil
	if _, err := e.ev.ApplyRestoreOnly(context.Background(), e.scope, "acc-1", "reconcile"); err != nil {
		t.Fatal(err)
	}
	if len(e.tasks.isolate) != 0 || len(e.tasks.restore) != 1 || e.tasks.restore[0] != "s-iso" {
		t.Fatalf("isolate=%v restore=%v, want restore of s-iso only", e.tasks.isolate, e.tasks.restore)
	}
}

func TestEvaluator_ApplyRestoreOnly_DryRunAndOffWriteNothing(t *testing.T) {
	for _, mode := range []string{ModeDryRun, ModeOff} {
		e := newEvalEnv(t, mode)
		e.sec.rows = []domain.PPPSecret{secretRow("s-iso", domain.IsolationIsolated, true, false)}
		if _, err := e.ev.ApplyRestoreOnly(context.Background(), e.scope, "acc-1", "reconcile"); err != nil {
			t.Fatal(err)
		}
		if len(e.tasks.isolate)+len(e.tasks.restore) != 0 {
			t.Fatalf("mode %s wrote tasks", mode)
		}
	}
}

func TestReconciler_RestoreOnly_OnlyIsolatedAccounts(t *testing.T) {
	rec, src, sec := newRecEnv(t, []string{"a", "b"}, []string{"b", "c"})
	res, err := rec.RunOrganizationRestoreOnly(context.Background(), testScope(t))
	if err != nil {
		t.Fatal(err)
	}
	if src.pages != 0 {
		t.Fatalf("trigger accounts listed %d times, want 0", src.pages)
	}
	if len(sec.seen) != 2 || res.Accounts != 2 {
		t.Fatalf("seen=%v res=%+v, want b,c only", sec.seen, res)
	}
}

func TestTaskProcessor_RestoreOnly_CancelsIsolateTasksWithoutRouterWrite(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t) // isolate task from an earlier entitled period

	res, err := e.p.RunRestoreOnly(context.Background(), e.scope, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res != (ProcessResult{Cancelled: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if e.clientA.WriteCalls() != 0 || routerProfile(e.clientA, "alice") != "paket-10m" {
		t.Fatal("router must not be touched")
	}
	ts := e.tasks.forSecret("s-1")
	if len(ts) != 1 || ts[0].Status != domain.TaskCancelled || ts[0].LastError != cancelEntitlementDisabled {
		t.Fatalf("tasks = %+v, want one cancelled %q (no isolate re-enqueued by follow-up)", ts, cancelEntitlementDisabled)
	}
	if e.srepo.byID("s-1").IsolationState != domain.IsolationNormal || len(e.cust.calls) != 0 {
		t.Fatal("no state change or notification expected")
	}
}

func TestTaskProcessor_RestoreOnly_StillExecutesRestore(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationIsolated, isolirProfile)
	e.enqueue(t) // no invoices: restore
	r := e.rrepo.routers[e.routerA]
	r.IsolirReady = false
	e.rrepo.routers[e.routerA] = r

	res, err := e.p.RunRestoreOnly(context.Background(), e.scope, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res != (ProcessResult{Done: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if routerProfile(e.clientA, "alice") != "paket-10m" || e.srepo.byID("s-1").IsolationState != domain.IsolationNormal {
		t.Fatal("restore must run in restore-only mode")
	}
}
