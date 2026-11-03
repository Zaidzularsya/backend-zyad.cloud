//go:build mikrotiklab

package service

import (
	"context"
	"net/netip"
	"os"
	"sync"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

// Live control-center test (plan 2026-11-02 Task 9): tenant modes, pilot test
// window and pause + restore-all against the real lab router and CPE.
//
//	set -a; . ~/.mikrotik-lab.env; set +a
//	MIKROTIK_LAB_WRITE=1 MIKROTIK_LAB_LIVE_USER='10mbps@debug' \
//	  go test -tags mikrotiklab -run TestLabControlCenter -v -count=1 -timeout 25m ./internal/modules/mikrotik/service/
//
// The router side is real (SetupService, Executor, RouterService). The DB side
// is the in-memory fakes of the unit tests, wired exactly like production:
// ControlService -> Evaluator -> task repo -> TaskProcessor -> Executor. A
// shared fake clock stands in for time: the 10 minute pilot window is crossed
// by advancing the clock, not by waiting (the production constant is untouched).
// Only the single named secret is touched; its profile is put back by cleanup.

// labControlRepo adds an APPROXIMATION of the MarkPilotRoundTrip rule to the
// in-memory control repo. The real SQL needs test_started_at set and an isolate
// task `done` with finished_at >= test_started_at; this wrapper only checks for
// a done isolate task created by the pilot_test event. The real SQL is covered
// by the integration test TestControlRepo_MarkPilotRoundTrip, not by this lab.
type labControlRepo struct {
	*fakeControlRepo
	tasks *fakeTaskRepo
}

func (r *labControlRepo) MarkPilotRoundTrip(_ context.Context, _ coretenant.Scope, accountID string) (bool, error) {
	r.tasks.mu.Lock()
	defer r.tasks.mu.Unlock()
	for _, t := range r.tasks.tasks {
		s := r.tasks.secrets.byID(t.SecretID)
		if s != nil && s.AccountID != nil && *s.AccountID == accountID &&
			t.Action == domain.ActionIsolate && t.Status == domain.TaskDone && t.CreatedByEvent == "pilot_test" {
			if r.ctl.PilotVerifiedAt == nil {
				at := r.now()
				r.ctl.PilotVerifiedAt = &at
			}
			return true, nil
		}
	}
	return false, nil
}

// labSecretRepo makes the fake report accounts with isolated secrets, which the
// reconciler needs (the plain fake returns none).
type labSecretRepo struct{ *fakeSecretRepo }

func (r labSecretRepo) AccountsWithIsolatedSecrets(context.Context, coretenant.Scope) ([]string, error) {
	var out []string
	for _, row := range r.rows {
		if row.IsolationState == domain.IsolationIsolated && row.AccountID != nil {
			out = append(out, *row.AccountID)
		}
	}
	return out, nil
}

func TestLabControlCenter(t *testing.T) {
	liveUser := os.Getenv("MIKROTIK_LAB_LIVE_USER")
	if liveUser == "" {
		t.Skip("MIKROTIK_LAB_LIVE_USER not set (PPPoE user of a connected lab CPE)")
	}
	env := newLabSetupEnv(t)
	ctx := context.Background()
	env.applyFresh(t)
	pool, err := netip.ParsePrefix(env.in.PoolCIDR)
	if err != nil {
		t.Fatal(err)
	}
	scope := testScope(t)

	rows, err := env.raw.Print(ctx, pppSecretPath(), pmk.Row{"name": liveUser})
	if err != nil || len(rows) != 1 || rows[0]["name"] != liveUser {
		t.Fatalf("secret %s: rows=%d err=%v", liveUser, len(rows), err)
	}
	secretID, origProfile, origRemote := rows[0][".id"], rows[0]["profile"], rows[0]["remote-address"]
	if origRemote != "" {
		t.Skipf("secret %s has its own remote-address; this test expects pool addressing", liveUser)
	}
	if origProfile == setupDefaultProf {
		t.Fatalf("secret already uses the isolir profile %q; refusing to run", setupDefaultProf)
	}
	before := liveSession(t, env.raw, liveUser)
	if before == nil {
		t.Skipf("no active session for %s; start the CPE first", liveUser)
	}
	if addrIn(pool, before["address"]) {
		t.Fatalf("session address already inside the isolir pool")
	}
	t.Logf("before: session id=%s address=%s profile=%q pool=%s", before[".id"], before["address"], origProfile, pool)

	// Always put the secret (and its session) back before the setup cleanup runs.
	t.Cleanup(func() {
		cur, err := env.raw.Print(context.Background(), pppSecretPath(), pmk.Row{"name": liveUser})
		if err != nil || len(cur) != 1 {
			t.Errorf("cleanup: read secret: rows=%d err=%v (restore %s manually: profile=%q)", len(cur), err, liveUser, origProfile)
			return
		}
		if cur[0]["profile"] != origProfile {
			if err := env.raw.Set(context.Background(), pppSecretPath(), secretID, pmk.Row{"profile": origProfile}); err != nil {
				t.Errorf("cleanup: restore secret %s: %v (restore manually: profile=%q)", liveUser, err, origProfile)
				return
			}
			if s := liveSession(t, env.raw, liveUser); s != nil && addrIn(pool, s["address"]) {
				_ = env.raw.Remove(context.Background(), pppActivePath, s[".id"])
			}
		}
	})

	// ---- in-memory side, wired like production ----
	const account = "lab-acc-1"
	var clockMu sync.Mutex
	clockBase := time.Now().UTC()
	var offset time.Duration
	clock := func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return clockBase.Add(offset) }
	advance := func(d time.Duration) { clockMu.Lock(); offset += d; clockMu.Unlock() }

	srepo := newFakeSecretRepo()
	acc := account
	srepo.rows[env.router.ID+"/"+liveUser] = &domain.PPPSecret{
		ID: "lab-s-1", OrganizationID: scope.OrganizationID(), RouterID: env.router.ID, Name: liveUser,
		Profile: origProfile, OriginalProfile: origProfile, Present: true, IsolationState: domain.IsolationNormal, AccountID: &acc,
	}
	trepo := newFakeTaskRepo(srepo)
	crepo := &labControlRepo{fakeControlRepo: newFakeControlRepo(domain.ModeSimulation), tasks: trepo}
	crepo.now = clock
	src := &evalSource{}
	ev := NewEvaluator(EvaluatorDeps{
		Source: src, Policy: &fakePolicyRepo{}, Secrets: srepo, Tasks: trepo, Now: clock, Mode: ModeOn, Control: crepo,
	})
	proc := NewTaskProcessor(TaskProcessorDeps{
		Tasks: trepo, Secrets: srepo, Routers: env.svc.routers, Evaluator: ev,
		Executor: NewExecutor(env.svc.routers), Locker: &fakeLocker{}, Audit: env.repo, Pilot: crepo, Now: clock,
	})
	audit := &captureAuditor{}
	ctl := NewControlService(ControlDeps{
		Control: crepo, Secrets: srepo, Tasks: trepo,
		Accounts: fakeAccounts{known: map[string]AccountRef{account: {ID: account, Name: "Lab Pilot"}}},
		Audit:    audit, Evaluator: ev, Now: clock,
	}, ModeOn)
	rec := NewReconciler(src, labSecretRepo{srepo}, ev, clock)

	drain := func(label string) ProcessResult {
		var sum ProcessResult
		for i := 0; i < 4; i++ {
			res, err := proc.RunOnce(ctx, scope, 10)
			if err != nil {
				t.Fatalf("%s: RunOnce: %v", label, err)
			}
			sum.Done += res.Done
			sum.Retried += res.Retried
			sum.Failed += res.Failed
			sum.Cancelled += res.Cancelled
			sum.Skipped += res.Skipped
			if res == (ProcessResult{}) {
				break
			}
		}
		t.Logf("%s: processor result %+v", label, sum)
		return sum
	}
	stateOf := func() string { return srepo.byID("lab-s-1").IsolationState }
	taskCount := func() int { trepo.mu.Lock(); defer trepo.mu.Unlock(); return len(trepo.tasks) }
	profileOnRouter := func() string {
		cur, err := env.raw.Print(ctx, pppSecretPath(), pmk.Row{"name": liveUser})
		if err != nil || len(cur) != 1 {
			t.Fatalf("read secret: rows=%d err=%v", len(cur), err)
		}
		return cur[0]["profile"]
	}
	// untouched: no task, DB state normal, router profile original, same session outside the pool.
	untouched := func(label string, wantTasks int, sessionID string) {
		t.Helper()
		if got := taskCount(); got != wantTasks {
			t.Errorf("%s: %d tasks, want %d", label, got, wantTasks)
		}
		if stateOf() != domain.IsolationNormal || profileOnRouter() != origProfile {
			t.Errorf("%s: secret state=%s router profile=%q, want normal/%q", label, stateOf(), profileOnRouter(), origProfile)
		}
		s := liveSession(t, env.raw, liveUser)
		if s == nil || s[".id"] != sessionID || addrIn(pool, s["address"]) {
			t.Errorf("%s: session changed or isolated: %v", label, s)
		}
	}
	waitIsolated := func(label, notID string) pmk.Row {
		s := waitSession(t, env.raw, liveUser, notID, func(r pmk.Row) bool { return addrIn(pool, r["address"]) })
		t.Logf("%s: CPE session %s address=%s inside isolir pool %s", label, s[".id"], s["address"], pool)
		if _, ok := isolirListEntries(t, env.raw)[s["address"]]; !ok {
			t.Errorf("%s: address %s not in address-list %s", label, s["address"], setupIsolirList)
		}
		return s
	}
	waitNormal := func(label, notID string) pmk.Row {
		s := waitSession(t, env.raw, liveUser, notID, func(r pmk.Row) bool { return !addrIn(pool, r["address"]) })
		t.Logf("%s: CPE session %s address=%s outside the isolir pool", label, s[".id"], s["address"])
		if stateOf() != domain.IsolationNormal || profileOnRouter() != origProfile {
			t.Errorf("%s: after restore state=%s profile=%q", label, stateOf(), profileOnRouter())
		}
		return s
	}

	sessID := before[".id"]
	overdue := []TriggerInvoice{{ID: "inv-lab", Number: "INV-LAB-1", Balance: "100000.00"}}

	// (a) tenant simulation + overdue candidate: nothing is queued or cut.
	src.invoices = overdue
	dec, err := ev.Apply(ctx, scope, account, "lab_sim")
	if err != nil {
		t.Fatalf("simulation Apply: %v", err)
	}
	t.Logf("(a) simulation: desired=%s isolateAllowed=%v reason=%q", dec.Desired, dec.IsolateAllowed, dec.BlockedReason)
	if dec.Desired != domain.IsolationIsolated || dec.IsolateAllowed || dec.BlockedReason != ReasonSimulation {
		t.Errorf("(a) decision = %+v, want desired isolated but blocked by simulation", dec)
	}
	drain("(a) simulation")
	time.Sleep(10 * time.Second)
	untouched("(a) simulation", 0, sessID)

	// (b1) pilot mode, overdue, account not in the pilot list: still nothing.
	if _, err := ctl.SetMode(ctx, scope, SetModeInput{Mode: domain.ModePilot}, "lab"); err != nil {
		t.Fatalf("SetMode pilot: %v", err)
	}
	dec, err = ev.Apply(ctx, scope, account, "lab_pilot_nonpilot")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("(b1) pilot mode, non-pilot overdue account: isolateAllowed=%v reason=%q", dec.IsolateAllowed, dec.BlockedReason)
	if dec.IsolateAllowed || dec.BlockedReason != ReasonNotInPilot {
		t.Errorf("(b1) decision = %+v, want blocked not_in_pilot", dec)
	}
	drain("(b1) pilot, not pilot")
	untouched("(b1) pilot, not pilot", 0, sessID)

	// (b2) the pilot account (no overdue invoice) isolates only during the test window.
	src.invoices = nil
	if err := ctl.AddPilot(ctx, scope, account, "lab"); err != nil {
		t.Fatalf("AddPilot: %v", err)
	}
	pilotRound := func(label string, endWith string) {
		t.Helper()
		started := clock()
		until, err := ctl.StartPilotTest(ctx, scope, account, "lab")
		if err != nil {
			t.Fatalf("%s: StartPilotTest: %v", label, err)
		}
		if !until.Equal(started.Add(10 * time.Minute)) {
			t.Errorf("%s: window ends %s, want start+10m (%s)", label, until, started.Add(10*time.Minute))
		}
		st, _ := ctl.State(ctx, scope)
		t.Logf("%s: test window %s -> %s; State.TestActiveCount=%d EffectiveMode=%s", label, started.Format(time.RFC3339), until.Format(time.RFC3339), st.TestActiveCount, st.EffectiveMode)
		if st.TestActiveCount != 1 {
			t.Errorf("%s: TestActiveCount = %d, want 1", label, st.TestActiveCount)
		}
		if res := drain(label + " isolate"); res.Done != 1 {
			t.Fatalf("%s: isolate task not done: %+v", label, res)
		}
		if stateOf() != domain.IsolationIsolated {
			t.Fatalf("%s: secret state = %s, want isolated", label, stateOf())
		}
		iso := waitIsolated(label, sessID)
		if crepo.ctl.PilotVerifiedAt != nil {
			t.Errorf("%s: pilot_verified_at set after isolate only", label)
		}
		switch endWith {
		case "expiry":
			advance(11 * time.Minute)
			st, _ = ctl.State(ctx, scope)
			t.Logf("%s: clock +11m; TestActiveCount=%d (window expired)", label, st.TestActiveCount)
			if st.TestActiveCount != 0 {
				t.Errorf("%s: window still active after expiry", label)
			}
			if _, err := rec.RunOrganization(ctx, scope); err != nil {
				t.Fatalf("%s: reconcile: %v", label, err)
			}
		case "stop":
			if err := ctl.StopPilotTest(ctx, scope, account, "lab"); err != nil {
				t.Fatalf("%s: StopPilotTest: %v", label, err)
			}
		}
		if res := drain(label + " restore"); res.Done != 1 {
			t.Fatalf("%s: restore task not done: %+v", label, res)
		}
		sessID = waitNormal(label, iso[".id"])[".id"]
		if crepo.ctl.PilotVerifiedAt == nil {
			t.Errorf("%s: pilot_verified_at not set after isolate + restore", label)
		} else {
			t.Logf("%s: pilot_verified_at set (%s)", label, crepo.ctl.PilotVerifiedAt.Format(time.RFC3339))
		}
	}
	pilotRound("(b2) pilot test, window expiry", "expiry")
	crepo.ctl.PilotVerifiedAt = nil // test-state reset to observe the explicit-stop path from scratch
	pilotRound("(b3) pilot test, StopPilotTest", "stop")

	// (c) paused: a new overdue candidate is not isolated.
	src.invoices = overdue
	if _, err := ctl.SetMode(ctx, scope, SetModeInput{Mode: domain.ModePaused}, "lab"); err != nil {
		t.Fatalf("SetMode paused: %v", err)
	}
	tasksBefore := taskCount()
	dec, err = ev.Apply(ctx, scope, account, "lab_paused")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("(c1) paused + overdue: isolateAllowed=%v reason=%q", dec.IsolateAllowed, dec.BlockedReason)
	if dec.IsolateAllowed || dec.BlockedReason != ReasonPaused {
		t.Errorf("(c1) decision = %+v, want blocked paused", dec)
	}
	drain("(c1) paused")
	untouched("(c1) paused", tasksBefore, sessID)

	// (c2) activate (verified now) to get a real isolated customer, then pause and RestoreAll.
	if _, err := ctl.SetMode(ctx, scope, SetModeInput{Mode: domain.ModeActive}, "lab"); err != nil {
		t.Fatalf("SetMode active: %v", err)
	}
	if _, err := ev.Apply(ctx, scope, account, "lab_active"); err != nil {
		t.Fatal(err)
	}
	if res := drain("(c2) active isolate"); res.Done != 1 {
		t.Fatalf("(c2) active isolate not done: %+v", res)
	}
	iso := waitIsolated("(c2) active", sessID)

	if _, err := ctl.SetMode(ctx, scope, SetModeInput{Mode: domain.ModePaused}, "lab"); err != nil {
		t.Fatalf("SetMode paused: %v", err)
	}
	drain("(c2) paused with isolated customer")
	if stateOf() != domain.IsolationIsolated {
		t.Errorf("(c2) pause alone must not restore anyone; state=%s", stateOf())
	}
	res, err := ctl.RestoreAll(ctx, scope, "lab")
	if err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	t.Logf("(c3) RestoreAll (invoice still overdue, mode paused): %+v", res)
	if res.Enqueued != 1 {
		t.Errorf("(c3) RestoreAll enqueued %d, want 1", res.Enqueued)
	}
	pr := drain("(c3) restore-all")
	if pr.Done != 1 {
		t.Errorf("(c3) restore-all task outcome %+v, want 1 done (customer still isolated: state=%s)", pr, stateOf())
		trepo.mu.Lock()
		for _, tk := range trepo.tasks {
			t.Logf("(c3) task %s action=%s event=%s status=%s lastError=%q", tk.ID, tk.Action, tk.CreatedByEvent, tk.Status, tk.LastError)
		}
		trepo.mu.Unlock()
		return
	}
	sessID = waitNormal("(c3) restore-all", iso[".id"])[".id"]

	// (c4) still paused: re-evaluation does not isolate again.
	tasksBefore = taskCount()
	if _, err := ev.Apply(ctx, scope, account, "lab_paused_again"); err != nil {
		t.Fatal(err)
	}
	drain("(c4) paused again")
	untouched("(c4) paused again", tasksBefore, sessID)
}
