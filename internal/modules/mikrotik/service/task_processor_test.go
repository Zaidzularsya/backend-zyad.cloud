package service

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
	pmk "zyad.cloud/internal/platform/mikrotik"
)

// fakeTaskRepo mirrors the documented TaskRepository semantics: one active
// (pending/running) task per secret, lifecycle methods return ErrTaskNotFound
// when the task is no longer active.
type fakeTaskRepo struct {
	repository.TaskRepository
	mu      sync.Mutex
	secrets *fakeSecretRepo
	tasks   map[string]*domain.IsolationTask
	seq     int

	claimErr   error
	completeFn func(id string) error // optional injection before Complete
}

func newFakeTaskRepo(secrets *fakeSecretRepo) *fakeTaskRepo {
	return &fakeTaskRepo{secrets: secrets, tasks: map[string]*domain.IsolationTask{}}
}

func taskActive(t *domain.IsolationTask) bool {
	return t.Status == domain.TaskPending || t.Status == domain.TaskRunning
}

func (f *fakeTaskRepo) enqueue(secretID, event, action string, priority int, eligible bool, cancelAction string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !eligible {
		return false
	}
	for _, t := range f.tasks {
		if t.SecretID == secretID && t.Action == cancelAction && t.Status == domain.TaskPending {
			t.Status, t.LastError = domain.TaskCancelled, "superseded"
		}
	}
	for _, t := range f.tasks {
		if t.SecretID == secretID && taskActive(t) {
			return false
		}
	}
	f.seq++
	id := fmt.Sprintf("t-%d", f.seq)
	f.tasks[id] = &domain.IsolationTask{ID: id, SecretID: secretID, Action: action, Priority: priority,
		Status: domain.TaskPending, CreatedByEvent: event, NextAttemptAt: time.Time{}}
	return true
}

func (f *fakeTaskRepo) EnqueueIsolate(_ context.Context, _ coretenant.Scope, secretID, event string) (bool, error) {
	s := f.secrets.byID(secretID)
	ok := s != nil && s.AccountID != nil && s.Present && !s.NeedsOriginalProfile && s.IsolationState == domain.IsolationNormal
	return f.enqueue(secretID, event, domain.ActionIsolate, domain.PriorityIsolate, ok, domain.ActionRestore), nil
}

func (f *fakeTaskRepo) EnqueueRestore(_ context.Context, _ coretenant.Scope, secretID, event string) (bool, error) {
	s := f.secrets.byID(secretID)
	ok := s != nil && s.IsolationState == domain.IsolationIsolated
	return f.enqueue(secretID, event, domain.ActionRestore, domain.PriorityRestore, ok, domain.ActionIsolate), nil
}

func (f *fakeTaskRepo) ClaimBatch(_ context.Context, _ coretenant.Scope, limit int, now time.Time, lease time.Duration) ([]domain.IsolationTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	var due []*domain.IsolationTask
	for _, t := range f.tasks {
		if (t.Status == domain.TaskPending && !t.NextAttemptAt.After(now)) ||
			(t.Status == domain.TaskRunning && t.LockedUntil != nil && t.LockedUntil.Before(now)) {
			due = append(due, t)
		}
	}
	sort.Slice(due, func(i, j int) bool {
		if due[i].Priority != due[j].Priority {
			return due[i].Priority > due[j].Priority
		}
		return due[i].ID < due[j].ID
	})
	var out []domain.IsolationTask
	for i, t := range due {
		if i >= limit {
			break
		}
		until, started := now.Add(lease), now
		t.Status, t.LockedUntil, t.StartedAt = domain.TaskRunning, &until, &started
		out = append(out, *t)
	}
	return out, nil
}

func (f *fakeTaskRepo) active(id string) (*domain.IsolationTask, error) {
	t, ok := f.tasks[id]
	if !ok || !taskActive(t) {
		return nil, mikrotik.ErrTaskNotFound
	}
	return t, nil
}

func (f *fakeTaskRepo) Release(_ context.Context, _ coretenant.Scope, id string, next time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, err := f.active(id)
	if err != nil {
		return err
	}
	t.Status, t.LockedUntil, t.NextAttemptAt = domain.TaskPending, nil, next
	return nil
}

func (f *fakeTaskRepo) Complete(_ context.Context, _ coretenant.Scope, id string) error {
	if f.completeFn != nil {
		if err := f.completeFn(id); err != nil {
			return err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	t, err := f.active(id)
	if err != nil {
		return err
	}
	t.Status, t.LockedUntil = domain.TaskDone, nil
	return nil
}

func (f *fakeTaskRepo) RetryLater(_ context.Context, _ coretenant.Scope, id string, next time.Time, code, msg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, err := f.active(id)
	if err != nil {
		return err
	}
	t.Status, t.LockedUntil, t.NextAttemptAt = domain.TaskPending, nil, next
	t.Attempts++
	t.LastErrorCode, t.LastError = code, msg
	return nil
}

func (f *fakeTaskRepo) Fail(_ context.Context, _ coretenant.Scope, id, code, msg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, err := f.active(id)
	if err != nil {
		return err
	}
	t.Status, t.LockedUntil = domain.TaskFailed, nil
	t.LastErrorCode, t.LastError = code, msg
	return nil
}

func (f *fakeTaskRepo) Cancel(_ context.Context, _ coretenant.Scope, id, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return mikrotik.ErrTaskNotFound
	}
	if taskActive(t) {
		t.Status, t.LockedUntil, t.LastError = domain.TaskCancelled, nil, reason
	}
	return nil
}

func (f *fakeTaskRepo) MarkTenantNotified(_ context.Context, _ coretenant.Scope, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return mikrotik.ErrTaskNotFound
	}
	now := time.Now()
	t.TenantNotifiedAt = &now
	return nil
}

func (f *fakeTaskRepo) MarkCustomerNotified(_ context.Context, _ coretenant.Scope, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return mikrotik.ErrTaskNotFound
	}
	now := time.Now()
	t.CustomerNotifiedAt = &now
	return nil
}

func (f *fakeTaskRepo) get(id string) domain.IsolationTask {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *f.tasks[id]
}

// forSecret returns the tasks of a secret ordered by id.
func (f *fakeTaskRepo) forSecret(secretID string) []domain.IsolationTask {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.IsolationTask
	for _, t := range f.tasks {
		if t.SecretID == secretID {
			out = append(out, *t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

type customerCall struct {
	kind, accountID string
	invoices        int
}

type fakeCustomer struct {
	calls []customerCall
	err   error
}

func (c *fakeCustomer) NotifyIsolated(_ context.Context, _ coretenant.Scope, accountID string, inv []TriggerInvoice) error {
	c.calls = append(c.calls, customerCall{"isolated", accountID, len(inv)})
	return c.err
}

func (c *fakeCustomer) NotifyRestored(_ context.Context, _ coretenant.Scope, accountID string) error {
	c.calls = append(c.calls, customerCall{"restored", accountID, 0})
	return c.err
}

type fakeAlerter struct {
	events []TaskFailureEvent
	err    error
}

func (a *fakeAlerter) NotifyTaskFailed(_ context.Context, _ coretenant.Scope, e TaskFailureEvent) error {
	a.events = append(a.events, e)
	return a.err
}

// hostFactory hands out one FakeClient per router host, so routers can fail
// independently.
type hostFactory struct {
	clients map[string]*pmk.FakeClient
}

func (f *hostFactory) Detect(context.Context, pmk.Endpoint) (string, string, error) {
	return pmk.APITypeREST, "7.15", nil
}

func (f *hostFactory) New(_ string, ep pmk.Endpoint) (pmk.RouterClient, error) {
	c, ok := f.clients[ep.Host]
	if !ok {
		return nil, errors.New("no client for host")
	}
	return c, nil
}

// panicExecutor panics for one secret and delegates otherwise.
type panicExecutor struct {
	TaskExecutor
	secretID string
}

func (p panicExecutor) Isolate(ctx context.Context, scope coretenant.Scope, s domain.PPPSecret) (string, error) {
	if s.ID == p.secretID {
		panic("boom")
	}
	return p.TaskExecutor.Isolate(ctx, scope, s)
}

const procAccount = "acc-1"

type procEnv struct {
	p       *TaskProcessor
	deps    TaskProcessorDeps
	tasks   *fakeTaskRepo
	srepo   *fakeSecretRepo
	rrepo   *fakeRepo
	routers *RouterService
	factory *hostFactory
	locker  *fakeLocker
	src     *evalSource
	pol     *fakePolicyRepo
	ev      *Evaluator
	cust    *fakeCustomer
	alert   *fakeAlerter
	scope   coretenant.Scope
	now     time.Time
	routerA string
	clientA *pmk.FakeClient
}

func newProcEnv(t *testing.T) *procEnv {
	t.Helper()
	e := &procEnv{
		rrepo:   newFakeRepo(),
		factory: &hostFactory{clients: map[string]*pmk.FakeClient{}},
		srepo:   newFakeSecretRepo(),
		locker:  &fakeLocker{},
		src:     &evalSource{},
		pol:     &fakePolicyRepo{},
		cust:    &fakeCustomer{},
		alert:   &fakeAlerter{},
		scope:   testScope(t),
		now:     time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC),
	}
	e.tasks = newFakeTaskRepo(e.srepo)
	e.routers = NewRouterService(e.rrepo, e.factory, testSecret, []netip.Prefix{netip.MustParsePrefix("10.8.0.0/24")})
	e.routerA, e.clientA = e.addRouter(t, "Core", "10.8.0.2")
	clock := func() time.Time { return e.now }
	e.ev = NewEvaluator(EvaluatorDeps{Source: e.src, Policy: e.pol, Secrets: e.srepo, Tasks: e.tasks, Now: clock, Mode: ModeOn})
	e.deps = TaskProcessorDeps{
		Tasks: e.tasks, Secrets: e.srepo, Routers: e.routers, Evaluator: e.ev,
		Executor: NewExecutor(e.routers), Locker: e.locker, Customer: e.cust, Alert: e.alert,
		Audit: e.rrepo, Now: clock,
	}
	e.p = NewTaskProcessor(e.deps)
	return e
}

func (e *procEnv) rebuild() { e.p = NewTaskProcessor(e.deps) }

func (e *procEnv) addRouter(t *testing.T, name, host string) (string, *pmk.FakeClient) {
	t.Helper()
	client := pmk.NewFakeClient()
	e.factory.clients[host] = client
	in := validInput()
	in.Name, in.Host = name, host
	r, err := e.routers.Register(context.Background(), e.scope, in)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	stored := e.rrepo.routers[r.ID]
	stored.IsolirReady, stored.IsolirProfileName = true, isolirProfile
	e.rrepo.routers[r.ID] = stored
	client.Seed("/ppp/profile", pmk.Row{"name": "paket-10m"}, pmk.Row{"name": isolirProfile})
	return r.ID, client
}

// addSecret stores a paired secret and seeds it on the router.
func (e *procEnv) addSecret(id, routerID, name, state, routerProfile string) {
	acc := procAccount
	e.srepo.rows[routerID+"/"+name] = &domain.PPPSecret{
		ID: id, OrganizationID: e.scope.OrganizationID(), RouterID: routerID, Name: name,
		Profile: routerProfile, OriginalProfile: "paket-10m", Present: true, IsolationState: state, AccountID: &acc,
	}
	for host, c := range e.factory.clients {
		if e.rrepo.routers[routerID].Host == host {
			c.Seed("/ppp/secret", pmk.Row{"name": name, "profile": routerProfile})
			c.Seed("/ppp/active", pmk.Row{"name": name})
		}
	}
}

func (e *procEnv) enqueue(t *testing.T) {
	t.Helper()
	if _, err := e.ev.Apply(context.Background(), e.scope, procAccount, "test"); err != nil {
		t.Fatalf("apply: %v", err)
	}
}

func (e *procEnv) run(t *testing.T) ProcessResult {
	t.Helper()
	res, err := e.p.RunOnce(context.Background(), e.scope, 10)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	return res
}

func (e *procEnv) audits(event string) []repository.AuditEntry {
	var out []repository.AuditEntry
	for _, a := range e.rrepo.audits {
		if a.Event == event {
			out = append(out, a)
		}
	}
	return out
}

func routerProfile(c *pmk.FakeClient, name string) string {
	for _, r := range c.Rows("/ppp/secret") {
		if r["name"] == name {
			return r["profile"]
		}
	}
	return ""
}

func onlyTask(t *testing.T, e *procEnv, secretID string) domain.IsolationTask {
	t.Helper()
	ts := e.tasks.forSecret(secretID)
	if len(ts) != 1 {
		t.Fatalf("tasks for %s = %+v", secretID, ts)
	}
	return ts[0]
}

func TestProcess_IsolateSuccess_MarksIsolatedAuditsNotifies(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)

	res := e.run(t)
	if res != (ProcessResult{Done: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if s := e.srepo.byID("s-1"); s.IsolationState != domain.IsolationIsolated || s.OriginalProfile != "paket-10m" {
		t.Fatalf("secret = %+v", s)
	}
	if got := routerProfile(e.clientA, "alice"); got != isolirProfile {
		t.Fatalf("router profile = %q", got)
	}
	task := onlyTask(t, e, "s-1")
	if task.Status != domain.TaskDone || task.CustomerNotifiedAt == nil {
		t.Fatalf("task = %+v", task)
	}
	au := e.audits("isolation.isolated")
	if len(au) != 1 || au[0].TargetID != e.routerA || au[0].ActorUserID != "" ||
		au[0].Metadata["target"] != "task" || au[0].Metadata["task_id"] != task.ID ||
		au[0].Metadata["secret_id"] != "s-1" || au[0].Metadata["action"] != "isolate" || au[0].Metadata["attempts"] != 1 {
		t.Fatalf("audit = %+v", au)
	}
	if len(e.cust.calls) != 1 || e.cust.calls[0] != (customerCall{"isolated", procAccount, 1}) {
		t.Fatalf("customer calls = %+v", e.cust.calls)
	}
	if len(e.locker.keys) != 1 || e.locker.keys[0] != "mikrotik:router:"+e.routerA || e.locker.released != 1 {
		t.Fatalf("locker = %+v", e.locker)
	}
}

func TestProcess_SupersededByPayment_CancelsWithoutTouchingRouter(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	e.src.invoices = nil // paid before the worker ran

	res := e.run(t)
	if res != (ProcessResult{Cancelled: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if e.clientA.WriteCalls() != 0 || routerProfile(e.clientA, "alice") != "paket-10m" {
		t.Fatal("router must not be touched")
	}
	task := onlyTask(t, e, "s-1")
	if task.Status != domain.TaskCancelled || task.LastError != "superseded" {
		t.Fatalf("task = %+v", task)
	}
	if e.srepo.byID("s-1").IsolationState != domain.IsolationNormal || len(e.cust.calls) != 0 {
		t.Fatal("no state change or notification expected")
	}
	if len(e.audits("task.cancelled")) != 1 || e.locker.released != e.locker.acquired {
		t.Fatalf("audits = %+v locker = %+v", e.rrepo.audits, e.locker)
	}
}

func TestProcess_UnpairedBeforeRun_Cancels(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	if err := e.srepo.Unpair(context.Background(), e.scope, "s-1"); err != nil {
		t.Fatal(err)
	}

	res := e.run(t)
	if res != (ProcessResult{Cancelled: 1}) || e.clientA.WriteCalls() != 0 {
		t.Fatalf("result = %+v writes = %d", res, e.clientA.WriteCalls())
	}
	if task := onlyTask(t, e, "s-1"); task.Status != domain.TaskCancelled || task.LastError != "superseded" {
		t.Fatalf("task = %+v", task)
	}
}

func TestProcess_IsolateOnRouterNoLongerReady_CancelsWithoutTouchingRouter(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	r := e.rrepo.routers[e.routerA]
	r.IsolirReady = false
	e.rrepo.routers[e.routerA] = r

	res := e.run(t)
	if res != (ProcessResult{Cancelled: 1}) || e.clientA.WriteCalls() != 0 {
		t.Fatalf("result = %+v writes = %d", res, e.clientA.WriteCalls())
	}
}

func TestProcess_RestoreOnRouterNotReady_StillRestores(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationIsolated, isolirProfile)
	e.enqueue(t) // no invoices: restore
	r := e.rrepo.routers[e.routerA]
	r.IsolirReady = false
	e.rrepo.routers[e.routerA] = r

	res := e.run(t)
	if res != (ProcessResult{Done: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if routerProfile(e.clientA, "alice") != "paket-10m" || e.srepo.byID("s-1").IsolationState != domain.IsolationNormal {
		t.Fatal("restore must run regardless of isolir_ready")
	}
	if len(e.audits("isolation.restored")) != 1 || len(e.cust.calls) != 1 || e.cust.calls[0].kind != "restored" {
		t.Fatalf("audits = %+v calls = %+v", e.rrepo.audits, e.cust.calls)
	}
}

func TestProcess_RouterBusy_ReleasesWithoutAttemptIncrement(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	e.locker.held = true

	res := e.run(t)
	if res != (ProcessResult{Skipped: 1}) || e.clientA.WriteCalls() != 0 {
		t.Fatalf("result = %+v writes = %d", res, e.clientA.WriteCalls())
	}
	task := onlyTask(t, e, "s-1")
	if task.Status != domain.TaskPending || task.Attempts != 0 || !task.NextAttemptAt.Equal(e.now.Add(15*time.Second)) {
		t.Fatalf("task = %+v", task)
	}
}

func TestProcess_Unreachable_RetriesThenFailsAfterMax_AlertsOnce(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	e.clientA.Err = fmt.Errorf("dial 10.8.0.2: %w (password %s)", pmk.ErrUnreachable, testPassword)

	var total ProcessResult
	for i := 1; i <= isolateMaxAttempts; i++ {
		res := e.run(t)
		total.Retried += res.Retried
		total.Failed += res.Failed
		wantAlerts := 0
		if i >= isolateNotifyAttempts {
			wantAlerts = 1
		}
		if len(e.alert.events) != wantAlerts {
			t.Fatalf("attempt %d: alerts = %d, want %d", i, len(e.alert.events), wantAlerts)
		}
		e.now = e.now.Add(2 * time.Hour)
	}
	if total.Retried != isolateMaxAttempts-1 || total.Failed != 1 {
		t.Fatalf("total = %+v", total)
	}
	task := onlyTask(t, e, "s-1")
	if task.Status != domain.TaskFailed || task.LastErrorCode != "unreachable" || task.TenantNotifiedAt == nil {
		t.Fatalf("task = %+v", task)
	}
	ev := e.alert.events[0]
	if ev.TaskID != task.ID || ev.RouterName != "Core" || ev.SecretName != "alice" || ev.Action != "isolate" || ev.Reason == "" {
		t.Fatalf("alert = %+v", ev)
	}
	for _, s := range []string{task.LastError, ev.Reason} {
		if strings.Contains(s, testPassword) || strings.Contains(s, "10.8.0.2") {
			t.Fatalf("leaked raw error: %q", s)
		}
	}
	if au := e.audits("task.failed"); len(au) != 1 || au[0].Metadata["attempts"] != isolateMaxAttempts {
		t.Fatalf("task.failed audits = %+v", au)
	}
	if e.srepo.byID("s-1").IsolationState != domain.IsolationNormal {
		t.Fatal("state must not change on failure")
	}
}

func TestProcess_RestoreWhileRunningIsolate_FollowUpRestoreEnqueuedAfterComplete(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	// The customer pays while the isolate is on the router: the payment's
	// evaluation cannot enqueue a restore because the isolate is still active.
	paid := false
	e.clientA.WriteHook = func(op, path string, _ pmk.Row) error {
		if op == "set" && !paid {
			paid = true
			e.src.invoices = nil
			e.enqueue(t)
			if n := len(e.tasks.forSecret("s-1")); n != 1 {
				t.Errorf("restore must not be enqueued while isolate runs, tasks = %d", n)
			}
		}
		return nil
	}

	if res := e.run(t); res != (ProcessResult{Done: 1}) {
		t.Fatalf("first run = %+v", res)
	}
	ts := e.tasks.forSecret("s-1")
	if len(ts) != 2 || ts[1].Action != domain.ActionRestore || ts[1].Status != domain.TaskPending || ts[1].Priority != domain.PriorityRestore {
		t.Fatalf("tasks = %+v", ts)
	}
	if res := e.run(t); res != (ProcessResult{Done: 1}) {
		t.Fatalf("second run = %+v", res)
	}
	if routerProfile(e.clientA, "alice") != "paket-10m" || e.srepo.byID("s-1").IsolationState != domain.IsolationNormal {
		t.Fatal("customer must be restored")
	}
}

func TestProcess_NotifierError_DoesNotFailTask(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	e.cust.err = errors.New("smtp down for alice@example.com")

	res := e.run(t)
	if res != (ProcessResult{Done: 1}) {
		t.Fatalf("result = %+v", res)
	}
	task := onlyTask(t, e, "s-1")
	if task.Status != domain.TaskDone || task.CustomerNotifiedAt == nil {
		t.Fatalf("task = %+v", task)
	}
	au := e.audits("notification.failed")
	if len(au) != 1 || au[0].Metadata["target"] != "task" || au[0].Metadata["task_id"] != task.ID {
		t.Fatalf("audit = %+v", au)
	}
	for _, v := range au[0].Metadata {
		if s, ok := v.(string); ok && strings.Contains(s, "example.com") {
			t.Fatalf("PII in audit: %+v", au[0].Metadata)
		}
	}
}

func TestProcess_RestoreProfileMissing_FailsAlerts(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationIsolated, isolirProfile)
	e.srepo.byID("s-1").OriginalProfile = "paket-hilang"
	e.enqueue(t)

	res := e.run(t)
	if res != (ProcessResult{Failed: 1}) || e.clientA.WriteCalls() != 0 {
		t.Fatalf("result = %+v writes = %d", res, e.clientA.WriteCalls())
	}
	task := onlyTask(t, e, "s-1")
	if task.Status != domain.TaskFailed || task.LastErrorCode != "profile_missing" || task.TenantNotifiedAt == nil {
		t.Fatalf("task = %+v", task)
	}
	if len(e.alert.events) != 1 || e.alert.events[0].Action != "restore" || e.alert.events[0].SecretName != "alice" {
		t.Fatalf("alerts = %+v", e.alert.events)
	}
	if len(e.audits("task.failed")) != 1 || e.srepo.byID("s-1").IsolationState != domain.IsolationIsolated {
		t.Fatal("expected task.failed audit and unchanged state")
	}
	// A failed task is not re-enqueued in the same pass (no hot loop).
	if n := len(e.tasks.forSecret("s-1")); n != 1 {
		t.Fatalf("tasks = %d", n)
	}
}

func TestProcess_TwoSecretsOneRouterDown_OtherCompletes(t *testing.T) {
	e := newProcEnv(t)
	routerB, clientB := e.addRouter(t, "Edge", "10.8.0.3")
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.addSecret("s-2", routerB, "alice-2", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	clientB.Err = pmk.ErrUnreachable

	res := e.run(t)
	if res != (ProcessResult{Done: 1, Retried: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if e.srepo.byID("s-1").IsolationState != domain.IsolationIsolated || routerProfile(e.clientA, "alice") != isolirProfile {
		t.Fatal("live router must be isolated")
	}
	t2 := onlyTask(t, e, "s-2")
	if e.srepo.byID("s-2").IsolationState != domain.IsolationNormal || t2.Status != domain.TaskPending || t2.Attempts != 1 {
		t.Fatalf("down router task = %+v", t2)
	}
}

func TestProcess_TaskCancelledWhileRunning_SecretStateFollowsRouter(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	var taskID string
	e.tasks.completeFn = func(id string) error {
		taskID = id
		return e.tasks.Cancel(context.Background(), e.scope, id, "by admin")
	}

	res, err := e.p.RunOnce(context.Background(), e.scope, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res != (ProcessResult{Done: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if e.srepo.byID("s-1").IsolationState != domain.IsolationIsolated {
		t.Fatal("DB must follow the router even when the task was lost")
	}
	if e.tasks.get(taskID).Status != domain.TaskCancelled {
		t.Fatal("cancelled task stays cancelled")
	}
}

func TestProcess_StateChangedBeforeMark_CancelsStateChanged(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	e.clientA.WriteHook = func(op, _ string, _ pmk.Row) error {
		if op == "set" {
			e.srepo.byID("s-1").IsolationState = domain.IsolationIsolated
		}
		return nil
	}

	res := e.run(t)
	if res != (ProcessResult{Cancelled: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if task := onlyTask(t, e, "s-1"); task.Status != domain.TaskCancelled || task.LastError != "state changed" {
		t.Fatalf("task = %+v", task)
	}
}

func TestProcess_SecretMissingOnRouter_MarksAbsentAndFails(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	e.srepo.byID("s-1").Name = "ghost"

	res := e.run(t)
	if res != (ProcessResult{Failed: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if e.srepo.byID("s-1").Present {
		t.Fatal("secret must be marked absent")
	}
	if task := onlyTask(t, e, "s-1"); task.LastErrorCode != "secret_missing" || len(e.alert.events) != 1 {
		t.Fatalf("task = %+v alerts = %+v", task, e.alert.events)
	}
}

func TestProcess_PanicInOneTask_ContinuesBatch(t *testing.T) {
	e := newProcEnv(t)
	routerB, _ := e.addRouter(t, "Edge", "10.8.0.3")
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.addSecret("s-2", routerB, "alice-2", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	e.deps.Executor = panicExecutor{TaskExecutor: NewExecutor(e.routers), secretID: "s-1"}
	e.rebuild()

	res, err := e.p.RunOnce(context.Background(), e.scope, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Done != 1 || e.srepo.byID("s-2").IsolationState != domain.IsolationIsolated {
		t.Fatalf("result = %+v", res)
	}
	// The panicked task keeps its lease and is reclaimed after it expires.
	if task := onlyTask(t, e, "s-1"); task.Status != domain.TaskRunning {
		t.Fatalf("panicked task = %+v", task)
	}
	if e.locker.acquired != e.locker.released {
		t.Fatalf("locks leaked: %+v", e.locker)
	}
}

func TestProcess_ClaimError_ReturnsError(t *testing.T) {
	e := newProcEnv(t)
	e.tasks.claimErr = errors.New("db down")
	if _, err := e.p.RunOnce(context.Background(), e.scope, 10); err == nil {
		t.Fatal("want error")
	}
}

func TestProcess_ContextCancelled_ReturnsError(t *testing.T) {
	e := newProcEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.p.RunOnce(ctx, e.scope, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestProcess_NilNotifiers_Skipped(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationIsolated, isolirProfile)
	e.srepo.byID("s-1").OriginalProfile = "paket-hilang"
	e.enqueue(t)
	e.deps.Customer, e.deps.Alert = nil, nil
	e.rebuild()
	if res := e.run(t); res != (ProcessResult{Failed: 1}) {
		t.Fatalf("result = %+v", res)
	}
}

// failSessionRemoval makes /ppp/active removal fail after the profile Set.
func failSessionRemoval(c *pmk.FakeClient, err error) {
	c.WriteHook = func(op, path string, _ pmk.Row) error {
		if op == "remove" && path == "/ppp/active" {
			return err
		}
		return nil
	}
}

func TestProcess_IsolatePartialThenPermanentFail_MarksIsolatedAndEnqueuesRestoreWhenPaid(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	// The customer pays while the isolate is on the router; then session
	// removal fails permanently after the profile was already changed.
	e.clientA.WriteHook = func(op, path string, _ pmk.Row) error {
		switch {
		case op == "set":
			e.src.invoices = nil
		case op == "remove" && path == "/ppp/active":
			return pmk.ErrAuthFailed
		}
		return nil
	}

	res := e.run(t)
	if res != (ProcessResult{Failed: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if routerProfile(e.clientA, "alice") != isolirProfile {
		t.Fatal("precondition: router changed")
	}
	s := e.srepo.byID("s-1")
	if s.IsolationState != domain.IsolationIsolated || s.OriginalProfile != "paket-10m" {
		t.Fatalf("DB must mirror the router: %+v", s)
	}
	au := e.audits("isolation.isolated")
	if len(au) != 1 || au[0].Metadata["partial"] != true || au[0].TargetID != e.routerA {
		t.Fatalf("audit = %+v", au)
	}
	if len(e.audits("task.failed")) != 1 || len(e.alert.events) != 1 {
		t.Fatal("task.failed audit and tenant alert are kept")
	}
	ts := e.tasks.forSecret("s-1")
	if len(ts) != 2 || ts[0].Status != domain.TaskFailed || ts[1].Action != domain.ActionRestore || ts[1].Status != domain.TaskPending ||
		ts[1].CreatedByEvent != "task_failed_partial" {
		t.Fatalf("tasks = %+v", ts)
	}
}

func TestProcess_IsolateFailsBeforeSet_DoesNotMarkIsolated(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	e.clientA.WriteHook = func(op, _ string, _ pmk.Row) error {
		if op == "set" {
			return pmk.ErrAuthFailed
		}
		return nil
	}

	if res := e.run(t); res != (ProcessResult{Failed: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if e.srepo.byID("s-1").IsolationState != domain.IsolationNormal || len(e.audits("isolation.isolated")) != 0 {
		t.Fatal("must not mark isolated when the router did not change")
	}
	if n := len(e.tasks.forSecret("s-1")); n != 1 {
		t.Fatalf("tasks = %d", n)
	}
}

func TestProcess_IsolatePartialTransient_DoesNotMarkIsolatedYet(t *testing.T) {
	e := newProcEnv(t)
	e.addSecret("s-1", e.routerA, "alice", domain.IsolationNormal, "paket-10m")
	e.src.invoices = oneInvoice
	e.enqueue(t)
	failSessionRemoval(e.clientA, pmk.ErrUnreachable)

	if res := e.run(t); res != (ProcessResult{Retried: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if e.srepo.byID("s-1").IsolationState != domain.IsolationNormal || len(e.audits("isolation.isolated")) != 0 {
		t.Fatal("a retry finishes the job; no state change yet")
	}
	// The retry completes idempotently once the router recovers.
	e.clientA.WriteHook = nil
	e.now = e.now.Add(time.Hour)
	if res := e.run(t); res != (ProcessResult{Done: 1}) {
		t.Fatalf("retry = %+v", res)
	}
	if s := e.srepo.byID("s-1"); s.IsolationState != domain.IsolationIsolated || s.OriginalProfile != "paket-10m" {
		t.Fatalf("secret = %+v", s)
	}
}
