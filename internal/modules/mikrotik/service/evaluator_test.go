package service

import (
	"context"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

type evalSource struct {
	invoices []TriggerInvoice
	today    time.Time
}

func (s *evalSource) TriggerInvoices(_ context.Context, _ coretenant.Scope, _ string, today time.Time) ([]TriggerInvoice, error) {
	s.today = today
	return s.invoices, nil
}
func (s *evalSource) AccountsWithTriggerInvoices(context.Context, coretenant.Scope, time.Time, string, int) ([]string, error) {
	return nil, nil
}
func (s *evalSource) AccountOfInvoice(context.Context, coretenant.Scope, string) (string, error) {
	return "", nil
}

type evalSecrets struct {
	repository.SecretRepository
	rows []domain.PPPSecret
}

func (s *evalSecrets) ListByAccount(context.Context, coretenant.Scope, string) ([]domain.PPPSecret, error) {
	return s.rows, nil
}

type evalTasks struct {
	repository.TaskRepository
	isolate, restore []string
	events           []string
	failFor          string
}

func (t *evalTasks) EnqueueIsolate(_ context.Context, _ coretenant.Scope, id, event string) (bool, error) {
	if id == t.failFor {
		return false, errBoom
	}
	t.isolate = append(t.isolate, id)
	t.events = append(t.events, event)
	return true, nil
}
func (t *evalTasks) EnqueueRestore(_ context.Context, _ coretenant.Scope, id, event string) (bool, error) {
	t.restore = append(t.restore, id)
	t.events = append(t.events, event)
	return true, nil
}

type boomErr struct{}

func (boomErr) Error() string { return "boom" }

var errBoom = boomErr{}

func secretRow(id, state string, present, needs bool) domain.PPPSecret {
	acc := "acc-1"
	return domain.PPPSecret{ID: id, AccountID: &acc, IsolationState: state, Present: present, NeedsOriginalProfile: needs}
}

type evalEnv struct {
	ev    *Evaluator
	src   *evalSource
	sec   *evalSecrets
	tasks *evalTasks
	pol   *fakePolicyRepo
	scope coretenant.Scope
}

func newEvalEnv(t *testing.T, mode string) *evalEnv {
	t.Helper()
	scope := testScope(t)
	e := &evalEnv{src: &evalSource{}, sec: &evalSecrets{}, tasks: &evalTasks{}, pol: &fakePolicyRepo{}, scope: scope}
	e.ev = NewEvaluator(EvaluatorDeps{
		Source: e.src, Policy: e.pol, Secrets: e.sec, Tasks: e.tasks, Mode: mode,
		Now: func() time.Time { return time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC) },
	})
	return e
}

var oneInvoice = []TriggerInvoice{{ID: "inv-1", Number: "INV-1", Balance: "100000.00"}}

func TestDecide_TriggerInvoiceAndNotExempt_Isolated(t *testing.T) {
	e := newEvalEnv(t, ModeOn)
	e.src.invoices = oneInvoice
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationNormal, true, false)}
	d, err := e.ev.Decide(context.Background(), e.scope, "acc-1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Desired != domain.IsolationIsolated || d.Exempt || len(d.Invoices) != 1 {
		t.Fatalf("decision: %+v", d)
	}
	if len(d.Secrets) != 1 || d.Secrets[0].Action != "isolate" {
		t.Fatalf("secrets: %+v", d.Secrets)
	}
	// 2026-10-31 20:00 UTC is already 1 Nov in WIB; today passed to source is the business day.
	if !e.src.today.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("today = %v", e.src.today)
	}
}

func TestDecide_Exempt_Normal(t *testing.T) {
	e := newEvalEnv(t, ModeOn)
	e.src.invoices = oneInvoice
	e.pol.rows = map[string]domain.CustomerPolicy{"acc-1": {AccountID: "acc-1", Exempt: true}}
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationNormal, true, false)}
	d, _ := e.ev.Decide(context.Background(), e.scope, "acc-1")
	if d.Desired != domain.IsolationNormal || !d.Exempt || d.Secrets[0].Action != "" {
		t.Fatalf("decision: %+v", d)
	}
}

func TestDecide_NoTriggerInvoice_Normal(t *testing.T) {
	e := newEvalEnv(t, ModeOn)
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationNormal, true, false)}
	d, _ := e.ev.Decide(context.Background(), e.scope, "acc-1")
	if d.Desired != domain.IsolationNormal || d.Secrets[0].Action != "" {
		t.Fatalf("decision: %+v", d)
	}
}

func TestDecide_UnpairedAccountNoSecrets_NoActions(t *testing.T) {
	e := newEvalEnv(t, ModeOn)
	e.src.invoices = oneInvoice
	d, _ := e.ev.Decide(context.Background(), e.scope, "acc-1")
	if d.Desired != domain.IsolationIsolated || len(d.Secrets) != 0 {
		t.Fatalf("decision: %+v", d)
	}
}

func TestDecide_TwoSecretsTwoRouters_ActionsPerSecret(t *testing.T) {
	e := newEvalEnv(t, ModeOn)
	e.src.invoices = oneInvoice
	e.sec.rows = []domain.PPPSecret{
		secretRow("s1", domain.IsolationNormal, true, false),
		secretRow("s2", domain.IsolationIsolated, true, false),
		secretRow("s3", domain.IsolationNormal, false, false), // absent: not considered
		secretRow("s4", domain.IsolationNormal, true, true),   // needs original profile: not considered
	}
	d, _ := e.ev.Decide(context.Background(), e.scope, "acc-1")
	got := map[string]string{}
	for _, a := range d.Secrets {
		got[a.SecretID] = a.Action
	}
	if got["s1"] != "isolate" || got["s2"] != "" || got["s3"] != "" || got["s4"] != "" {
		t.Fatalf("actions: %+v", got)
	}
}

func TestApply_DryRunWritesNothing(t *testing.T) {
	e := newEvalEnv(t, ModeDryRun)
	e.src.invoices = oneInvoice
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationNormal, true, false)}
	d, err := e.ev.Apply(context.Background(), e.scope, "acc-1", "test")
	if err != nil || d.Desired != domain.IsolationIsolated {
		t.Fatalf("%+v %v", d, err)
	}
	if len(e.tasks.isolate)+len(e.tasks.restore) != 0 {
		t.Fatalf("dry run enqueued: %+v", e.tasks)
	}
}

func TestApply_OffNoop(t *testing.T) {
	for _, mode := range []string{ModeOff, "", "bogus"} {
		e := newEvalEnv(t, mode)
		e.src.invoices = oneInvoice
		e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationNormal, true, false)}
		d, err := e.ev.Apply(context.Background(), e.scope, "acc-1", "test")
		if err != nil || d.AccountID != "" || len(d.Secrets) != 0 || len(e.tasks.isolate) != 0 {
			t.Fatalf("mode %q: %+v %v %+v", mode, d, err, e.tasks)
		}
	}
}

func TestApply_IsolatedNoTrigger_EnqueuesRestore(t *testing.T) {
	e := newEvalEnv(t, ModeOn)
	e.sec.rows = []domain.PPPSecret{
		secretRow("s1", domain.IsolationIsolated, false, false), // restore even if absent
		secretRow("s2", domain.IsolationNormal, true, false),
	}
	d, err := e.ev.Apply(context.Background(), e.scope, "acc-1", "invoice.paid")
	if err != nil || d.Desired != domain.IsolationNormal {
		t.Fatalf("%+v %v", d, err)
	}
	if len(e.tasks.restore) != 1 || e.tasks.restore[0] != "s1" || len(e.tasks.isolate) != 0 || e.tasks.events[0] != "invoice.paid" {
		t.Fatalf("tasks: %+v", e.tasks)
	}
}

func TestApply_EnqueueErrorDoesNotStopOthers(t *testing.T) {
	e := newEvalEnv(t, ModeOn)
	e.src.invoices = oneInvoice
	e.tasks.failFor = "s1"
	e.sec.rows = []domain.PPPSecret{
		secretRow("s1", domain.IsolationNormal, true, false),
		secretRow("s2", domain.IsolationNormal, true, false),
	}
	_, err := e.ev.Apply(context.Background(), e.scope, "acc-1", "x")
	if err == nil || len(e.tasks.isolate) != 1 || e.tasks.isolate[0] != "s2" {
		t.Fatalf("err=%v tasks=%+v", err, e.tasks)
	}
}
