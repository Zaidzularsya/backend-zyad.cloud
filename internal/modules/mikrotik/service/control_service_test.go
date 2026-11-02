package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

// ctlNow is 2 Nov 2026 10:00 WIB.
var ctlNow = time.Date(2026, 11, 2, 3, 0, 0, 0, time.UTC)

// fakeControlRepo is an in-memory repository.ControlRepository.
type fakeControlRepo struct {
	ctl       domain.ControlSettings
	pilots    map[string]*domain.PilotAccount
	policies  map[string]repository.Exemption
	writes    int
	activates int
	now       func() time.Time
}

func newFakeControlRepo(mode string) *fakeControlRepo {
	return &fakeControlRepo{
		ctl:      domain.ControlSettings{Mode: mode},
		pilots:   map[string]*domain.PilotAccount{},
		policies: map[string]repository.Exemption{},
		now:      func() time.Time { return ctlNow },
	}
}

func (f *fakeControlRepo) Get(context.Context, coretenant.Scope) (domain.ControlSettings, error) {
	return f.ctl, nil
}
func (f *fakeControlRepo) SetGraceDays(_ context.Context, _ coretenant.Scope, days int, _ string) (domain.ControlSettings, error) {
	if days < 0 || days > domain.MaxGraceDays {
		return domain.ControlSettings{}, &mikrotik.InvalidInputError{Message: "grace days must be between 0 and 60"}
	}
	f.writes++
	f.ctl.GraceDays = days
	return f.ctl, nil
}
func (f *fakeControlRepo) SetMode(_ context.Context, _ coretenant.Scope, mode, _ string) (domain.ControlSettings, error) {
	f.writes++
	f.ctl.Mode = mode
	return f.ctl, nil
}
func (f *fakeControlRepo) ActivateWithExemptions(_ context.Context, _ coretenant.Scope, mode string, ex []repository.Exemption, _ string) (domain.ControlSettings, error) {
	f.writes++
	f.activates++
	for _, e := range ex {
		f.policies[e.AccountID] = e
	}
	f.ctl.Mode = mode
	return f.ctl, nil
}
func (f *fakeControlRepo) ListPilot(context.Context, coretenant.Scope) ([]domain.PilotAccount, error) {
	var out []domain.PilotAccount
	for _, p := range f.pilots {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AccountID < out[j].AccountID })
	return out, nil
}
func (f *fakeControlRepo) PilotState(_ context.Context, _ coretenant.Scope, accountID string) (bool, bool, error) {
	p, ok := f.pilots[accountID]
	if !ok {
		return false, false, nil
	}
	return true, p.TestUntil != nil && p.TestUntil.After(f.now()), nil
}
func (f *fakeControlRepo) AddPilot(_ context.Context, _ coretenant.Scope, accountID, _ string) error {
	if _, ok := f.pilots[accountID]; !ok {
		f.pilots[accountID] = &domain.PilotAccount{AccountID: accountID, AddedAt: f.now()}
	}
	return nil
}
func (f *fakeControlRepo) RemovePilot(_ context.Context, _ coretenant.Scope, accountID string) error {
	delete(f.pilots, accountID)
	return nil
}
func (f *fakeControlRepo) StartPilotTest(_ context.Context, _ coretenant.Scope, accountID string, until time.Time) error {
	p, ok := f.pilots[accountID]
	if !ok {
		return mikrotik.ErrAccountNotFound
	}
	started := f.now()
	p.TestStartedAt, p.TestUntil = &started, &until
	return nil
}
func (f *fakeControlRepo) StopPilotTest(_ context.Context, _ coretenant.Scope, accountID string) error {
	p, ok := f.pilots[accountID]
	if !ok {
		return mikrotik.ErrAccountNotFound
	}
	end := f.now()
	p.TestUntil = &end
	return nil
}
func (f *fakeControlRepo) MarkPilotRoundTrip(context.Context, coretenant.Scope, string) (bool, error) {
	return false, nil
}

// ctlSecrets serves ListByAccount and a keyset ListIsolated over sorted ids.
type ctlSecrets struct {
	repository.SecretRepository
	byAccount map[string][]domain.PPPSecret
	isolated  []string
	limits    []int
}

func (s *ctlSecrets) ListByAccount(_ context.Context, _ coretenant.Scope, accountID string) ([]domain.PPPSecret, error) {
	return s.byAccount[accountID], nil
}

func (s *ctlSecrets) ListIsolated(_ context.Context, _ coretenant.Scope, afterID string, limit int) ([]domain.PPPSecret, error) {
	s.limits = append(s.limits, limit)
	var out []domain.PPPSecret
	for _, id := range s.isolated {
		if id > afterID && len(out) < limit {
			out = append(out, domain.PPPSecret{ID: id, IsolationState: domain.IsolationIsolated})
		}
	}
	return out, nil
}

// ctlTasks: EnqueueRestore refuses ids in cooldown and ids with an active task;
// an enqueued id becomes active.
type ctlTasks struct {
	repository.TaskRepository
	cooldown map[string]bool
	failFor  map[string]bool
	active   map[string]bool
	isolate  []string
	restore  []string
	events   []string
}

func newCtlTasks() *ctlTasks {
	return &ctlTasks{cooldown: map[string]bool{}, failFor: map[string]bool{}, active: map[string]bool{}}
}

func (t *ctlTasks) EnqueueRestore(_ context.Context, _ coretenant.Scope, id, event string) (bool, error) {
	if t.failFor[id] {
		return false, errBoom
	}
	if t.cooldown[id] || t.active[id] {
		return false, nil
	}
	t.active[id] = true
	t.restore = append(t.restore, id)
	t.events = append(t.events, event)
	return true, nil
}

func (t *ctlTasks) EnqueueIsolate(_ context.Context, _ coretenant.Scope, id, event string) (bool, error) {
	t.isolate = append(t.isolate, id)
	t.events = append(t.events, event)
	return true, nil
}

type ctlEnv struct {
	svc   *ControlService
	repo  *fakeControlRepo
	sec   *ctlSecrets
	tasks *ctlTasks
	audit *captureAuditor
	scope coretenant.Scope
}

const (
	accPilot   = "acc-pilot"
	accOther   = "acc-other"
	accForeign = "acc-foreign"
)

func newCtlEnv(t *testing.T, engine, mode string) *ctlEnv {
	t.Helper()
	repo := newFakeControlRepo(mode)
	sec := &ctlSecrets{byAccount: map[string][]domain.PPPSecret{}}
	tasks := newCtlTasks()
	audit := &captureAuditor{}
	now := func() time.Time { return ctlNow }
	var eval *Evaluator
	if engine != ModeOff {
		eval = NewEvaluator(EvaluatorDeps{
			Source: &evalSource{}, Policy: &fakePolicyRepo{}, Secrets: sec, Tasks: tasks,
			Mode: engine, Now: now, Control: repo,
		})
	}
	accounts := fakeAccounts{known: map[string]AccountRef{
		accPilot: {ID: accPilot, Name: "Pilot Sendiri"},
		accOther: {ID: accOther, Name: "Budi"},
	}}
	svc := NewControlService(ControlDeps{
		Control: repo, Secrets: sec, Tasks: tasks, Accounts: accounts, Audit: audit, Evaluator: eval, Now: now,
	}, engine)
	return &ctlEnv{svc: svc, repo: repo, sec: sec, tasks: tasks, audit: audit, scope: testScope(t)}
}

func (e *ctlEnv) events() []string {
	var out []string
	for _, a := range e.audit.entries {
		out = append(out, a.Event)
	}
	return out
}

func TestSetMode_ActiveRequiresPilotVerified(t *testing.T) {
	for _, from := range []string{domain.ModeSimulation, domain.ModePilot, domain.ModePaused} {
		e := newCtlEnv(t, ModeOn, from)
		_, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModeActive}, "u1")
		if !errors.Is(err, mikrotik.ErrPilotRequired) {
			t.Fatalf("from %s: err = %v, want ErrPilotRequired", from, err)
		}
		if e.repo.writes != 0 || e.repo.ctl.Mode != from || len(e.audit.entries) != 0 {
			t.Fatalf("from %s: nothing may be written: writes=%d mode=%s audit=%v", from, e.repo.writes, e.repo.ctl.Mode, e.events())
		}
	}
}

func TestSetMode_ActiveAfterVerifiedFromAnyMode(t *testing.T) {
	for _, from := range []string{domain.ModeSimulation, domain.ModePilot, domain.ModePaused} {
		e := newCtlEnv(t, ModeOn, from)
		v := ctlNow.Add(-time.Hour)
		e.repo.ctl.PilotVerifiedAt = &v
		st, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModeActive}, "u1")
		if err != nil {
			t.Fatalf("from %s: %v", from, err)
		}
		if st.Mode != domain.ModeActive || st.EffectiveMode != domain.ModeActive {
			t.Fatalf("from %s: state = %+v", from, st)
		}
		if len(e.audit.entries) != 1 || e.audit.entries[0].Event != "control.mode_changed" {
			t.Fatalf("audit = %v", e.events())
		}
		md := e.audit.entries[0].Metadata
		if md["from"] != from || md["to"] != domain.ModeActive || md["exemptions"] != 0 {
			t.Fatalf("metadata = %v", md)
		}
	}
	// Every other transition is allowed without a verified pilot.
	for _, to := range []string{domain.ModeSimulation, domain.ModePilot, domain.ModePaused} {
		e := newCtlEnv(t, ModeOn, domain.ModeActive)
		if _, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: to}, "u1"); err != nil || e.repo.ctl.Mode != to {
			t.Fatalf("active -> %s: err=%v mode=%s", to, err, e.repo.ctl.Mode)
		}
	}
	// Unknown mode.
	e := newCtlEnv(t, ModeOn, domain.ModePilot)
	if _, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: "turbo"}, "u1"); !errors.Is(err, mikrotik.ErrModeInvalid) {
		t.Fatalf("unknown mode err = %v", err)
	}
	// No-op transition returns the state without writing.
	if st, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModePilot}, "u1"); err != nil || st.Mode != domain.ModePilot || e.repo.writes != 0 {
		t.Fatalf("no-op: st=%+v err=%v writes=%d", st, err, e.repo.writes)
	}
}

func TestSetMode_ExemptionsOnlyWithActive(t *testing.T) {
	e := newCtlEnv(t, ModeOn, domain.ModeSimulation)
	v := ctlNow
	e.repo.ctl.PilotVerifiedAt = &v
	ex := &ExemptionInput{AccountIDs: []string{accOther}, Reason: "pelanggan VIP"}
	_, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModePilot, Exemptions: ex}, "u1")
	if !errors.Is(err, mikrotik.ErrModeInvalid) || e.repo.writes != 0 {
		t.Fatalf("err = %v writes = %d", err, e.repo.writes)
	}
	until := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	ex.Until = &until
	ex.AccountIDs = []string{accOther, accOther, accPilot}
	st, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModeActive, Exemptions: ex}, "u1")
	if err != nil || st.Mode != domain.ModeActive {
		t.Fatalf("st=%+v err=%v", st, err)
	}
	if e.repo.activates != 1 || len(e.repo.policies) != 2 || e.repo.policies[accOther].Reason != "pelanggan VIP" || !e.repo.policies[accOther].Until.Equal(until) {
		t.Fatalf("activates=%d policies=%+v", e.repo.activates, e.repo.policies)
	}
	got := e.events()
	if len(got) != 3 || got[0] != "control.mode_changed" || got[1] != "customer_policy.exempt_set" || got[2] != "customer_policy.exempt_set" {
		t.Fatalf("audit = %v", got)
	}
	if e.audit.entries[0].Metadata["exemptions"] != 2 {
		t.Fatalf("metadata = %v", e.audit.entries[0].Metadata)
	}

	// Validation: reason required, until not in the past.
	e = newCtlEnv(t, ModeOn, domain.ModePilot)
	e.repo.ctl.PilotVerifiedAt = &v
	for _, bad := range []*ExemptionInput{
		{AccountIDs: []string{accOther}, Reason: "  "},
		{AccountIDs: []string{accOther}, Reason: "x", Until: ptrTime(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))},
	} {
		if _, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModeActive, Exemptions: bad}, "u1"); !errors.Is(err, mikrotik.ErrInvalidInput) {
			t.Fatalf("bad %+v: err = %v", bad, err)
		}
	}
	if e.repo.writes != 0 {
		t.Fatalf("invalid exemptions must not write")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestSetMode_ForeignAccountRejectsWholeRequest(t *testing.T) {
	e := newCtlEnv(t, ModeOn, domain.ModePilot)
	v := ctlNow
	e.repo.ctl.PilotVerifiedAt = &v
	ex := &ExemptionInput{AccountIDs: []string{accOther, accForeign}, Reason: "VIP"}
	_, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModeActive, Exemptions: ex}, "u1")
	if !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("err = %v", err)
	}
	if e.repo.writes != 0 || e.repo.ctl.Mode != domain.ModePilot || len(e.repo.policies) != 0 || len(e.audit.entries) != 0 {
		t.Fatalf("mode and policy must stay unchanged: writes=%d mode=%s policies=%v", e.repo.writes, e.repo.ctl.Mode, e.repo.policies)
	}
}

func TestSetMode_Over500Exemptions_422(t *testing.T) {
	e := newCtlEnv(t, ModeOn, domain.ModePilot)
	v := ctlNow
	e.repo.ctl.PilotVerifiedAt = &v
	ids := make([]string, 501)
	for i := range ids {
		ids[i] = fmt.Sprintf("acc-%03d", i)
	}
	_, err := e.svc.SetMode(context.Background(), e.scope, SetModeInput{Mode: domain.ModeActive, Exemptions: &ExemptionInput{AccountIDs: ids, Reason: "x"}}, "u1")
	if !errors.Is(err, mikrotik.ErrInvalidInput) || e.repo.writes != 0 {
		t.Fatalf("err = %v writes = %d", err, e.repo.writes)
	}
}

func TestRestoreAll_OnlyWhenPaused(t *testing.T) {
	for _, mode := range []string{domain.ModeSimulation, domain.ModePilot, domain.ModeActive} {
		e := newCtlEnv(t, ModeOn, mode)
		e.sec.isolated = []string{"s1"}
		if _, err := e.svc.RestoreAll(context.Background(), e.scope, "u1"); !errors.Is(err, mikrotik.ErrNotPaused) {
			t.Fatalf("mode %s: err = %v", mode, err)
		}
		if len(e.tasks.restore) != 0 || len(e.audit.entries) != 0 {
			t.Fatalf("mode %s: nothing may be enqueued", mode)
		}
	}
}

func TestRestoreAll_BatchesAndReportsSkipped(t *testing.T) {
	e := newCtlEnv(t, ModeOn, domain.ModePaused)
	for i := 0; i < 450; i++ {
		id := fmt.Sprintf("s%04d", i)
		e.sec.isolated = append(e.sec.isolated, id)
		if i%15 == 0 { // 30 in cooldown
			e.tasks.cooldown[id] = true
		}
	}
	e.tasks.failFor["s0001"] = true
	res, err := e.svc.RestoreAll(context.Background(), e.scope, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Enqueued != 419 || res.Skipped != 30 || res.Failed != 1 {
		t.Fatalf("result = %+v, want enqueued=419 skipped=30 failed=1", res)
	}
	if len(e.sec.limits) != 3 || e.sec.limits[0] != 200 {
		t.Fatalf("pages = %v, want 3 pages of 200", e.sec.limits)
	}
	for _, ev := range e.tasks.events {
		if ev != "restore_all" {
			t.Fatalf("event = %s", ev)
		}
	}
	if len(e.audit.entries) != 1 || e.audit.entries[0].Event != "control.restore_all" ||
		e.audit.entries[0].Metadata["enqueued"] != 419 || e.audit.entries[0].Metadata["skipped"] != 30 || e.audit.entries[0].Metadata["failed"] != 1 {
		t.Fatalf("audit = %+v", e.audit.entries)
	}

	// Without the failing secret: 450 isolated, 30 in cooldown.
	e = newCtlEnv(t, ModeOn, domain.ModePaused)
	for i := 0; i < 450; i++ {
		id := fmt.Sprintf("s%04d", i)
		e.sec.isolated = append(e.sec.isolated, id)
		if i%15 == 0 {
			e.tasks.cooldown[id] = true
		}
	}
	res, err = e.svc.RestoreAll(context.Background(), e.scope, "u1")
	if err != nil || res.Enqueued != 420 || res.Skipped != 30 || res.Failed != 0 {
		t.Fatalf("result = %+v err = %v, want enqueued=420 skipped=30", res, err)
	}
}

func TestRestoreAll_Idempotent(t *testing.T) {
	e := newCtlEnv(t, ModeOn, domain.ModePaused)
	e.sec.isolated = []string{"s1", "s2", "s3"}
	first, err := e.svc.RestoreAll(context.Background(), e.scope, "u1")
	if err != nil || first.Enqueued != 3 {
		t.Fatalf("first = %+v err = %v", first, err)
	}
	second, err := e.svc.RestoreAll(context.Background(), e.scope, "u1")
	if err != nil || second.Enqueued != 0 || second.Skipped != 3 {
		t.Fatalf("second = %+v err = %v", second, err)
	}
	if len(e.tasks.restore) != 3 {
		t.Fatalf("restores = %v", e.tasks.restore)
	}
}

func pairedSecret(id, account string) domain.PPPSecret {
	acc := account
	return domain.PPPSecret{ID: id, AccountID: &acc, IsolationState: domain.IsolationNormal, Present: true}
}

func TestStartPilotTest_Preconditions(t *testing.T) {
	ctx := context.Background()
	setup := func(engine, mode string) *ctlEnv {
		e := newCtlEnv(t, engine, mode)
		e.repo.pilots[accPilot] = &domain.PilotAccount{AccountID: accPilot, AddedAt: ctlNow}
		e.sec.byAccount[accPilot] = []domain.PPPSecret{pairedSecret("s-pilot", accPilot)}
		return e
	}
	// Engine off (no evaluator) and dry_run.
	for _, engine := range []string{ModeOff, ModeDryRun} {
		e := setup(engine, domain.ModePilot)
		if _, err := e.svc.StartPilotTest(ctx, e.scope, accPilot, "u1"); !errors.Is(err, mikrotik.ErrEngineOff) {
			t.Fatalf("engine %s: err = %v", engine, err)
		}
		if e.repo.pilots[accPilot].TestUntil != nil {
			t.Fatalf("engine %s: test must not start", engine)
		}
	}
	// Tenant mode must be pilot.
	for _, mode := range []string{domain.ModeSimulation, domain.ModeActive, domain.ModePaused} {
		e := setup(ModeOn, mode)
		if _, err := e.svc.StartPilotTest(ctx, e.scope, accPilot, "u1"); !errors.Is(err, mikrotik.ErrPilotModeRequired) {
			t.Fatalf("mode %s: err = %v", mode, err)
		}
	}
	// Not in the pilot list.
	e := setup(ModeOn, domain.ModePilot)
	if _, err := e.svc.StartPilotTest(ctx, e.scope, accOther, "u1"); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("non pilot: err = %v", err)
	}
	// No paired secret.
	e = setup(ModeOn, domain.ModePilot)
	e.sec.byAccount[accPilot] = nil
	if _, err := e.svc.StartPilotTest(ctx, e.scope, accPilot, "u1"); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("no secret: err = %v", err)
	}
	if len(e.tasks.isolate) != 0 {
		t.Fatal("no task may be enqueued on a failed precondition")
	}

	// Happy path: window set, then the evaluator enqueues the isolate.
	e = setup(ModeOn, domain.ModePilot)
	until, err := e.svc.StartPilotTest(ctx, e.scope, accPilot, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !until.Equal(ctlNow.Add(10 * time.Minute)) {
		t.Fatalf("until = %v", until)
	}
	if len(e.tasks.isolate) != 1 || e.tasks.isolate[0] != "s-pilot" || e.tasks.events[0] != "pilot_test" {
		t.Fatalf("isolate = %v events = %v", e.tasks.isolate, e.tasks.events)
	}
	if got := e.events(); len(got) != 1 || got[0] != "pilot.test_started" {
		t.Fatalf("audit = %v", got)
	}
}

func TestStopPilotTest(t *testing.T) {
	ctx := context.Background()
	e := newCtlEnv(t, ModeOn, domain.ModePilot)
	started, until := ctlNow.Add(-2*time.Minute), ctlNow.Add(8*time.Minute)
	e.repo.pilots[accPilot] = &domain.PilotAccount{AccountID: accPilot, TestStartedAt: &started, TestUntil: &until}
	s := pairedSecret("s-pilot", accPilot)
	s.IsolationState = domain.IsolationIsolated
	e.sec.byAccount[accPilot] = []domain.PPPSecret{s}

	if err := e.svc.StopPilotTest(ctx, e.scope, accPilot, "u1"); err != nil {
		t.Fatal(err)
	}
	if p := e.repo.pilots[accPilot]; p.TestUntil == nil || p.TestUntil.After(ctlNow) {
		t.Fatalf("test window must end now: %+v", p)
	}
	if len(e.tasks.restore) != 1 || e.tasks.events[0] != "pilot_test_stopped" {
		t.Fatalf("restore = %v events = %v", e.tasks.restore, e.tasks.events)
	}
	if got := e.events(); len(got) != 1 || got[0] != "pilot.test_stopped" {
		t.Fatalf("audit = %v", got)
	}
	// Not a pilot.
	if err := e.svc.StopPilotTest(ctx, e.scope, accOther, "u1"); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("non pilot: err = %v", err)
	}
}

func TestPilotAddRemove(t *testing.T) {
	ctx := context.Background()
	e := newCtlEnv(t, ModeOn, domain.ModePilot)
	if err := e.svc.AddPilot(ctx, e.scope, accForeign, "u1"); !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("foreign: err = %v", err)
	}
	if err := e.svc.AddPilot(ctx, e.scope, accPilot, "u1"); err != nil {
		t.Fatal(err)
	}
	list, err := e.svc.ListPilot(ctx, e.scope)
	if err != nil || len(list) != 1 || list[0].AccountName != "Pilot Sendiri" || list[0].TestActive {
		t.Fatalf("list = %+v err = %v", list, err)
	}
	// Remove with a running test: stopped first, then removed, then restored.
	until := ctlNow.Add(5 * time.Minute)
	e.repo.pilots[accPilot].TestStartedAt, e.repo.pilots[accPilot].TestUntil = &ctlNow, &until
	s := pairedSecret("s-pilot", accPilot)
	s.IsolationState = domain.IsolationIsolated
	e.sec.byAccount[accPilot] = []domain.PPPSecret{s}
	if err := e.svc.RemovePilot(ctx, e.scope, accPilot, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.repo.pilots[accPilot]; ok {
		t.Fatal("pilot must be removed")
	}
	if len(e.tasks.restore) != 1 {
		t.Fatalf("restore = %v", e.tasks.restore)
	}
	got := e.events()
	if len(got) != 3 || got[0] != "pilot.added" || got[1] != "pilot.test_stopped" || got[2] != "pilot.removed" {
		t.Fatalf("audit = %v", got)
	}
}

func TestControlState_EngineOff(t *testing.T) {
	ctx := context.Background()
	e := newCtlEnv(t, ModeOff, domain.ModeActive)
	e.repo.ctl.GraceDays = 3
	until := ctlNow.Add(time.Minute)
	e.repo.pilots[accPilot] = &domain.PilotAccount{AccountID: accPilot, TestUntil: &until}
	e.repo.pilots[accOther] = &domain.PilotAccount{AccountID: accOther}
	e.sec.isolated = []string{"s1", "s2"}
	st, err := e.svc.State(ctx, e.scope)
	if err != nil {
		t.Fatal(err)
	}
	if st.Engine != ModeOff || st.EffectiveMode != ModeOff || st.Mode != domain.ModeActive || st.GraceDays != 3 ||
		st.PilotCount != 2 || st.TestActiveCount != 1 || st.IsolatedSecrets != 2 {
		t.Fatalf("state = %+v", st)
	}
	if st, _ := newCtlEnv(t, ModeDryRun, domain.ModeActive).svc.State(ctx, e.scope); st.EffectiveMode != ModeDryRun || st.Engine != ModeDryRun {
		t.Fatalf("dry_run state = %+v", st)
	}
	if st, _ := newCtlEnv(t, ModeOn, domain.ModePaused).svc.State(ctx, e.scope); st.EffectiveMode != domain.ModePaused || st.Engine != ModeOn {
		t.Fatalf("on state = %+v", st)
	}
	// Unknown env value counts as off.
	if st, _ := newCtlEnv(t, "banana", domain.ModeActive).svc.State(ctx, e.scope); st.Engine != ModeOff {
		t.Fatalf("unknown env state = %+v", st)
	}
}

func TestSetGraceDays(t *testing.T) {
	e := newCtlEnv(t, ModeOn, domain.ModePilot)
	if _, err := e.svc.SetGraceDays(context.Background(), e.scope, 61, "u1"); !errors.Is(err, mikrotik.ErrInvalidInput) {
		t.Fatalf("err = %v", err)
	}
	st, err := e.svc.SetGraceDays(context.Background(), e.scope, 7, "u1")
	if err != nil || st.GraceDays != 7 {
		t.Fatalf("st = %+v err = %v", st, err)
	}
	if got := e.events(); len(got) != 1 || got[0] != "control.grace_days_changed" {
		t.Fatalf("audit = %v", got)
	}
}
