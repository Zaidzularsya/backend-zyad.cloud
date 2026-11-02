package service

import (
	"context"
	"sync"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

// fakeControl is an in-memory ControlReader.
type fakeControl struct {
	mu         sync.Mutex
	ctl        domain.ControlSettings
	pilot      map[string]bool
	testActive map[string]bool
	gets       int
}

func newFakeControl(mode string, grace int) *fakeControl {
	return &fakeControl{ctl: domain.ControlSettings{Mode: mode, GraceDays: grace}, pilot: map[string]bool{}, testActive: map[string]bool{}}
}

func (f *fakeControl) Get(context.Context, coretenant.Scope) (domain.ControlSettings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	return f.ctl, nil
}

func (f *fakeControl) PilotState(_ context.Context, _ coretenant.Scope, accountID string) (bool, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pilot[accountID], f.pilot[accountID] && f.testActive[accountID], nil
}

func (f *fakeControl) setMode(mode string) {
	f.mu.Lock()
	f.ctl.Mode = mode
	f.mu.Unlock()
}

func newEvalEnvWithControl(t *testing.T, mode string, ctl *fakeControl) *evalEnv {
	t.Helper()
	e := newEvalEnv(t, mode)
	e.ev.d.Control = ctl
	return e
}

func TestDecide_PilotTestForcesIsolation(t *testing.T) {
	ctl := newFakeControl(domain.ModePilot, 0)
	ctl.pilot["acc-1"], ctl.testActive["acc-1"] = true, true
	e := newEvalEnvWithControl(t, ModeOn, ctl)
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationNormal, true, false)}

	d, err := e.ev.Decide(context.Background(), e.scope, "acc-1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Desired != domain.IsolationIsolated || len(d.Invoices) != 0 || d.Secrets[0].Action != actionIsolate {
		t.Fatalf("pilot test without invoices must isolate: %+v", d)
	}
	if !d.IsolateAllowed || d.BlockedReason != "" {
		t.Fatalf("pilot account in pilot mode must be allowed: %+v", d)
	}

	// An effective exemption beats the pilot test.
	e.pol.rows = map[string]domain.CustomerPolicy{"acc-1": {AccountID: "acc-1", Exempt: true}}
	if d, _ = e.ev.Decide(context.Background(), e.scope, "acc-1"); d.Desired != domain.IsolationNormal {
		t.Fatalf("exempt account must stay normal: %+v", d)
	}

	// Test over: the isolated secret is restored.
	e.pol.rows = nil
	ctl.testActive["acc-1"] = false
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationIsolated, true, false)}
	if d, _ = e.ev.Decide(context.Background(), e.scope, "acc-1"); d.Desired != domain.IsolationNormal || d.Secrets[0].Action != actionRestore {
		t.Fatalf("finished pilot test must restore: %+v", d)
	}
}

func TestDecide_GraceShiftsCutoffAndExemptionExpires(t *testing.T) {
	// now = 2026-11-01 03:00 WIB.
	ctl := newFakeControl(domain.ModeActive, 3)
	e := newEvalEnvWithControl(t, ModeOn, ctl)
	e.src.invoices = oneInvoice
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationNormal, true, false)}
	if _, err := e.ev.Decide(context.Background(), e.scope, "acc-1"); err != nil {
		t.Fatal(err)
	}
	if !e.src.today.Equal(time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("cutoff passed to source = %v, want 2026-10-29", e.src.today)
	}

	expired := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	e.pol.rows = map[string]domain.CustomerPolicy{"acc-1": {AccountID: "acc-1", Exempt: true, ExemptedUntil: &expired}}
	d, _ := e.ev.Decide(context.Background(), e.scope, "acc-1")
	if d.Exempt || d.Desired != domain.IsolationIsolated {
		t.Fatalf("expired exemption must not protect: %+v", d)
	}
	until := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	e.pol.rows["acc-1"] = domain.CustomerPolicy{AccountID: "acc-1", Exempt: true, ExemptedUntil: &until}
	if d, _ = e.ev.Decide(context.Background(), e.scope, "acc-1"); !d.Exempt || d.Desired != domain.IsolationNormal {
		t.Fatalf("exemption until today must protect: %+v", d)
	}
}

func TestDecide_NilControlKeepsWave1Behavior(t *testing.T) {
	e := newEvalEnv(t, ModeOn)
	e.src.invoices = oneInvoice
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationNormal, true, false)}
	d, err := e.ev.Decide(context.Background(), e.scope, "acc-1")
	if err != nil {
		t.Fatal(err)
	}
	if !d.IsolateAllowed || d.BlockedReason != "" || d.Desired != domain.IsolationIsolated {
		t.Fatalf("nil control must behave as active: %+v", d)
	}
	if !e.src.today.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("nil control must use grace 0, today = %v", e.src.today)
	}
}

func TestApply_PausedDropsIsolateKeepsRestore(t *testing.T) {
	e := newEvalEnvWithControl(t, ModeOn, newFakeControl(domain.ModePaused, 0))
	e.src.invoices = oneInvoice
	e.sec.rows = []domain.PPPSecret{secretRow("s-normal", domain.IsolationNormal, true, false)}
	d, err := e.ev.Apply(context.Background(), e.scope, "acc-1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.tasks.isolate) != 0 || d.IsolateAllowed || d.BlockedReason != ReasonPaused {
		t.Fatalf("paused must not enqueue isolate: isolate=%v dec=%+v", e.tasks.isolate, d)
	}

	e.src.invoices = nil
	e.sec.rows = []domain.PPPSecret{secretRow("s-iso", domain.IsolationIsolated, true, false)}
	if _, err := e.ev.Apply(context.Background(), e.scope, "acc-1", "test"); err != nil {
		t.Fatal(err)
	}
	if len(e.tasks.restore) != 1 || e.tasks.restore[0] != "s-iso" {
		t.Fatalf("paused must still restore: restore=%v", e.tasks.restore)
	}
}

func TestApply_PilotModeOnlyPilotAccounts(t *testing.T) {
	ctl := newFakeControl(domain.ModePilot, 0)
	e := newEvalEnvWithControl(t, ModeOn, ctl)
	e.src.invoices = oneInvoice
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationNormal, true, false)}
	d, _ := e.ev.Apply(context.Background(), e.scope, "acc-1", "test")
	if len(e.tasks.isolate) != 0 || d.BlockedReason != ReasonNotInPilot {
		t.Fatalf("non-pilot account must not be isolated in pilot mode: %+v", d)
	}
	ctl.pilot["acc-1"] = true
	if _, err := e.ev.Apply(context.Background(), e.scope, "acc-1", "test"); err != nil {
		t.Fatal(err)
	}
	if len(e.tasks.isolate) != 1 {
		t.Fatalf("pilot account must be isolated: %v", e.tasks.isolate)
	}
}

func TestApply_SimulationLogsOnly(t *testing.T) {
	e := newEvalEnvWithControl(t, ModeOn, newFakeControl(domain.ModeSimulation, 0))
	e.src.invoices = oneInvoice
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationNormal, true, false)}
	d, err := e.ev.Apply(context.Background(), e.scope, "acc-1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.tasks.isolate)+len(e.tasks.restore) != 0 {
		t.Fatalf("simulation must not enqueue an isolate: isolate=%v restore=%v", e.tasks.isolate, e.tasks.restore)
	}
	if d.Desired != domain.IsolationIsolated || d.IsolateAllowed || d.BlockedReason != ReasonSimulation {
		t.Fatalf("decision: %+v", d)
	}

	// Restore is never blocked by the tenant mode, not even in simulation
	// (e.g. a tenant moved back to simulation with customers still isolated).
	e.src.invoices = nil
	e.sec.rows = []domain.PPPSecret{secretRow("s-iso", domain.IsolationIsolated, true, false)}
	if _, err := e.ev.Apply(context.Background(), e.scope, "acc-1", "test"); err != nil {
		t.Fatal(err)
	}
	if len(e.tasks.isolate) != 0 || len(e.tasks.restore) != 1 {
		t.Fatalf("simulation must still restore: isolate=%v restore=%v", e.tasks.isolate, e.tasks.restore)
	}
}

func TestReconciler_ReadsControlOnceAndUsesGraceCutoff(t *testing.T) {
	src := &recSource{accounts: []string{"a", "b"}}
	sec := &recSecrets{isolated: []string{"c"}}
	now := func() time.Time { return time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC) }
	ctl := newFakeControl(domain.ModeActive, 2)
	eval := NewEvaluator(EvaluatorDeps{Source: src, Policy: &fakePolicyRepo{}, Secrets: sec, Tasks: &evalTasks{}, Mode: ModeOn, Now: now, Control: ctl})
	src2 := &cutoffSource{recSource: src}
	eval.d.Source = src2
	rec := NewReconciler(src2, sec, eval, now)
	res, err := rec.RunOrganization(context.Background(), testScope(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.Accounts != 3 || ctl.gets != 1 {
		t.Fatalf("res=%+v control gets=%d, want 3 accounts and 1 read", res, ctl.gets)
	}
	want := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	if len(src2.cutoffs) == 0 {
		t.Fatal("no cutoff recorded")
	}
	for _, c := range src2.cutoffs {
		if !c.Equal(want) {
			t.Fatalf("cutoffs = %v, want all %v", src2.cutoffs, want)
		}
	}
}

// cutoffSource records every "today" passed to the source.
type cutoffSource struct {
	*recSource
	mu      sync.Mutex
	cutoffs []time.Time
}

func (s *cutoffSource) TriggerInvoices(ctx context.Context, scope coretenant.Scope, id string, today time.Time) ([]TriggerInvoice, error) {
	s.mu.Lock()
	s.cutoffs = append(s.cutoffs, today)
	s.mu.Unlock()
	return s.recSource.TriggerInvoices(ctx, scope, id, today)
}

func (s *cutoffSource) AccountsWithTriggerInvoices(ctx context.Context, scope coretenant.Scope, today time.Time, after string, limit int) ([]string, error) {
	s.mu.Lock()
	s.cutoffs = append(s.cutoffs, today)
	s.mu.Unlock()
	return s.recSource.AccountsWithTriggerInvoices(ctx, scope, today, after, limit)
}

// fakePilotRecorder mirrors MarkPilotRoundTrip: verified once a done isolate
// task exists for a secret of the account.
type fakePilotRecorder struct {
	tasks    *fakeTaskRepo
	calls    []string
	verified bool
}

func (f *fakePilotRecorder) MarkPilotRoundTrip(_ context.Context, _ coretenant.Scope, accountID string) (bool, error) {
	f.calls = append(f.calls, accountID)
	f.tasks.mu.Lock()
	defer f.tasks.mu.Unlock()
	for _, t := range f.tasks.tasks {
		s := f.tasks.secrets.byID(t.SecretID)
		if s != nil && s.AccountID != nil && *s.AccountID == accountID && t.Action == domain.ActionIsolate && t.Status == domain.TaskDone {
			f.verified = true
		}
	}
	return f.verified, nil
}

func newProcEnvWithControl(t *testing.T, ctl *fakeControl) (*procEnv, *fakePilotRecorder) {
	t.Helper()
	e := newProcEnv(t)
	e.ev.d.Control = ctl
	rec := &fakePilotRecorder{tasks: e.tasks}
	e.deps.Pilot = rec
	e.rebuild()
	return e, rec
}

func TestProcessor_CancelsQueuedIsolateWhenPaused(t *testing.T) {
	ctl := newFakeControl(domain.ModeActive, 0)
	e, _ := newProcEnvWithControl(t, ctl)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t) // queued while active
	ctl.setMode(domain.ModePaused)

	res := e.run(t)
	if res != (ProcessResult{Cancelled: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if e.clientA.WriteCalls() != 0 || routerProfile(e.clientA, "alice") != "paket-10m" {
		t.Fatal("router must not be written")
	}
	ts := e.tasks.forSecret("s-1")
	if len(ts) != 1 || ts[0].Status != domain.TaskCancelled || ts[0].LastError != cancelModeBlocked || cancelModeBlocked != "mode_blocked" {
		t.Fatalf("tasks = %+v, want one cancelled %q and no re-enqueue", ts, cancelModeBlocked)
	}
	if e.srepo.byID("s-1").IsolationState != domain.IsolationNormal || len(e.cust.calls) != 0 {
		t.Fatal("no state change or notification expected")
	}
}

func TestProcessor_CancelsNeverStartedIsolateWithoutLock(t *testing.T) {
	ctl := newFakeControl(domain.ModeSimulation, 0)
	e, _ := newProcEnvWithControl(t, ctl)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	task := domain.IsolationTask{ID: "t-x", SecretID: "s-1", Action: domain.ActionIsolate, Status: domain.TaskRunning}
	e.tasks.tasks["t-x"] = &task
	if out := e.p.process(context.Background(), e.scope, task); out != outcomeCancelled {
		t.Fatalf("outcome = %v", out)
	}
	if len(e.locker.keys) != 0 || e.clientA.WriteCalls() != 0 {
		t.Fatal("a never-started blocked isolate must be cancelled before locking the router")
	}
	if got := e.tasks.get("t-x"); got.LastError != cancelModeBlocked {
		t.Fatalf("task = %+v", got)
	}
}

func TestProcessor_RestoreRunsWhenPaused(t *testing.T) {
	ctl := newFakeControl(domain.ModePaused, 0)
	e, _ := newProcEnvWithControl(t, ctl)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationIsolated, isolirProfile)
	e.enqueue(t) // paid: restore

	res := e.run(t)
	if res != (ProcessResult{Done: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if routerProfile(e.clientA, "alice") != "paket-10m" || e.srepo.byID("s-1").IsolationState != domain.IsolationNormal {
		t.Fatal("restore must run while paused")
	}
}

func TestProcessor_PilotRoundTripSetsVerified(t *testing.T) {
	// Restore only (no isolate in this test window): not verified.
	ctl := newFakeControl(domain.ModePilot, 0)
	e, rec := newProcEnvWithControl(t, ctl)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationIsolated, isolirProfile)
	e.enqueue(t)
	if res := e.run(t); res != (ProcessResult{Done: 1}) {
		t.Fatalf("restore result = %+v", res)
	}
	if len(rec.calls) != 1 || rec.verified {
		t.Fatalf("restore only: calls=%v verified=%v, want 1 call, not verified", rec.calls, rec.verified)
	}

	// Pilot test: isolate (no invoices), stop the test, restore -> verified.
	ctl2 := newFakeControl(domain.ModePilot, 0)
	ctl2.pilot[procAccount], ctl2.testActive[procAccount] = true, true
	e2, rec2 := newProcEnvWithControl(t, ctl2)
	e2.addSecret("s-1", e2.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e2.enqueue(t)
	if res := e2.run(t); res != (ProcessResult{Done: 1}) {
		t.Fatalf("isolate result = %+v", res)
	}
	if routerProfile(e2.clientA, "alice") != isolirProfile || len(rec2.calls) != 0 {
		t.Fatalf("pilot test must isolate; recorder calls=%v", rec2.calls)
	}
	ctl2.mu.Lock()
	ctl2.testActive[procAccount] = false
	ctl2.mu.Unlock()
	e2.enqueue(t)
	if res := e2.run(t); res != (ProcessResult{Done: 1}) {
		t.Fatalf("restore result = %+v", res)
	}
	if routerProfile(e2.clientA, "alice") != "paket-10m" || !rec2.verified || len(rec2.calls) != 1 {
		t.Fatalf("round trip: calls=%v verified=%v", rec2.calls, rec2.verified)
	}
}
