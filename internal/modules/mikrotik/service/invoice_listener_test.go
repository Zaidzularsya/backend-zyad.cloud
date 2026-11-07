package service

import (
	"context"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

type invoiceOwner struct {
	evalSource
	owner map[string]string
}

func (s *invoiceOwner) AccountOfInvoice(_ context.Context, _ coretenant.Scope, id string) (string, error) {
	if a, ok := s.owner[id]; ok {
		return a, nil
	}
	return "", mikrotik.ErrAccountNotFound
}

// activeRestoreTasks mimics the partial unique index: one active restore per secret.
type dedupTasks struct {
	evalTasks
	active map[string]bool
}

func (t *dedupTasks) EnqueueRestore(ctx context.Context, scope coretenant.Scope, id, event string) (bool, error) {
	if t.active[id] {
		return false, nil
	}
	t.active[id] = true
	return t.evalTasks.EnqueueRestore(ctx, scope, id, event)
}

func newListenerEnv(t *testing.T) (*InvoiceListener, *invoiceOwner, *evalEnv, *dedupTasks) {
	t.Helper()
	e := newEvalEnv(t, ModeOn)
	src := &invoiceOwner{owner: map[string]string{"inv-1": "acc-1"}}
	tasks := &dedupTasks{active: map[string]bool{}}
	e.ev = NewEvaluator(EvaluatorDeps{Source: src, Policy: e.pol, Secrets: e.sec, Tasks: tasks, Mode: ModeOn, Now: func() time.Time { return time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC) }})
	return NewInvoiceListener(src, e.ev), src, e, tasks
}

func TestOnInvoiceChanged_EvaluatesOwningAccount(t *testing.T) {
	l, _, e, tasks := newListenerEnv(t)
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationIsolated, true, false)}
	l.OnInvoiceChanged(context.Background(), e.scope, "inv-1", "invoice_paid")
	if len(tasks.restore) != 1 || tasks.restore[0] != "s1" || tasks.events[0] != "invoice_paid" {
		t.Fatalf("tasks: %+v", tasks)
	}
}

func TestOnInvoiceChanged_UnknownInvoiceIgnored(t *testing.T) {
	l, _, e, tasks := newListenerEnv(t)
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationIsolated, true, false)}
	l.OnInvoiceChanged(context.Background(), e.scope, "inv-foreign", "invoice_paid")
	if len(tasks.restore)+len(tasks.isolate) != 0 {
		t.Fatalf("tasks: %+v", tasks)
	}
}

func TestOnInvoiceChanged_ReplayIdempotent(t *testing.T) {
	l, _, e, tasks := newListenerEnv(t)
	e.sec.rows = []domain.PPPSecret{secretRow("s1", domain.IsolationIsolated, true, false)}
	l.OnInvoiceChanged(context.Background(), e.scope, "inv-1", "invoice_paid")
	l.OnInvoiceChanged(context.Background(), e.scope, "inv-1", "invoice_paid")
	if len(tasks.restore) != 1 {
		t.Fatalf("restore tasks = %v", tasks.restore)
	}
}
