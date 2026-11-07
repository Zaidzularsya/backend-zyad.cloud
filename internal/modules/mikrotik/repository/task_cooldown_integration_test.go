//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
	"zyad.cloud/internal/modules/mikrotik/service"
)

// failTask enqueues (if needed), claims and fails the secret's active task,
// then backdates finished_at by age.
func failTask(t *testing.T, f isoFixture, action string, age time.Duration) string {
	t.Helper()
	repo := taskRepo(f.e)
	var ok bool
	var err error
	if action == domain.ActionIsolate {
		ok, err = repo.EnqueueIsolate(f.e.ctx, f.scope, f.secretID, "x")
	} else {
		ok, err = repo.EnqueueRestore(f.e.ctx, f.scope, f.secretID, "x")
	}
	if err != nil || !ok {
		t.Fatalf("enqueue %s: %v %v", action, ok, err)
	}
	claimed, err := repo.ClaimBatch(f.e.ctx, f.scope, 1, time.Now().UTC().Add(time.Second), time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	if err := repo.Fail(f.e.ctx, f.scope, claimed[0].ID, "auth_failed", "denied"); err != nil {
		t.Fatal(err)
	}
	if age > 0 {
		secretScopedTx(t, f.e, f.scope, func(tx pgx.Tx) {
			if _, err := tx.Exec(f.e.ctx, `UPDATE mikrotik_isolation_tasks SET finished_at = now() - $2 * interval '1 second'
				WHERE id = $1`, claimed[0].ID, int64(age/time.Second)); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(f.e.ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
	return claimed[0].ID
}

func TestEnqueueIsolate_RefusedWithinFailCooldown(t *testing.T) {
	f := newIsoFixture(t)
	failTask(t, f, domain.ActionIsolate, 0)
	if ok, err := taskRepo(f.e).EnqueueIsolate(f.e.ctx, f.scope, f.secretID, "reconcile"); err != nil || ok {
		t.Fatalf("within cooldown: ok=%v err=%v", ok, err)
	}
}

func TestEnqueueIsolate_AllowedAfterFailCooldown(t *testing.T) {
	f := newIsoFixture(t)
	failTask(t, f, domain.ActionIsolate, domain.IsolateFailCooldown+time.Minute)
	if ok, err := taskRepo(f.e).EnqueueIsolate(f.e.ctx, f.scope, f.secretID, "reconcile"); err != nil || !ok {
		t.Fatalf("after cooldown: ok=%v err=%v", ok, err)
	}
}

func TestEnqueueRestore_CooldownAndOppositeActionNotBlocked(t *testing.T) {
	f := newIsoFixture(t)
	repo := taskRepo(f.e)
	// A failed isolate does not block a restore.
	failTask(t, f, domain.ActionIsolate, 0)
	if ok, err := secretRepo(f.e).MarkIsolated(f.e.ctx, f.scope, f.secretID, ""); err != nil || !ok {
		t.Fatal(ok, err)
	}
	failTask(t, f, domain.ActionRestore, 0) // enqueue succeeds despite the failed isolate
	if ok, err := repo.EnqueueRestore(f.e.ctx, f.scope, f.secretID, "reconcile"); err != nil || ok {
		t.Fatalf("restore within cooldown: ok=%v err=%v", ok, err)
	}

	g := newIsoFixture(t)
	if ok, err := secretRepo(g.e).MarkIsolated(g.e.ctx, g.scope, g.secretID, ""); err != nil || !ok {
		t.Fatal(ok, err)
	}
	failTask(t, g, domain.ActionRestore, domain.RestoreFailCooldown+time.Minute)
	if ok, err := taskRepo(g.e).EnqueueRestore(g.e.ctx, g.scope, g.secretID, "reconcile"); err != nil || !ok {
		t.Fatalf("restore after cooldown: ok=%v err=%v", ok, err)
	}
}

func TestRetry_UnaffectedByFailCooldown(t *testing.T) {
	f := newIsoFixture(t)
	id := failTask(t, f, domain.ActionIsolate, 0)
	got, err := taskRepo(f.e).Retry(f.e.ctx, f.scope, id)
	if err != nil || got.Status != domain.TaskPending {
		t.Fatalf("retry: %v %+v", err, got)
	}
}

// --- worker-level: permanent failure is not re-enqueued every tick ---

type oneOverdue struct{ account string }

func (o oneOverdue) TriggerInvoices(context.Context, coretenant.Scope, string, time.Time) ([]service.TriggerInvoice, error) {
	return []service.TriggerInvoice{{ID: "inv-1", Number: "INV-1", DueDate: time.Now().AddDate(0, 0, -30), Balance: "100000"}}, nil
}

func (o oneOverdue) AccountsWithTriggerInvoices(_ context.Context, _ coretenant.Scope, _ time.Time, after string, _ int) ([]string, error) {
	if after != "" {
		return nil, nil
	}
	return []string{o.account}, nil
}

func (o oneOverdue) AccountOfInvoice(context.Context, coretenant.Scope, string) (string, error) {
	return o.account, nil
}

// authFailingExecutor fails every router step permanently.
type authFailingExecutor struct{}

func (authFailingExecutor) Isolate(context.Context, coretenant.Scope, domain.PPPSecret) (string, error) {
	return "", mikrotik.ErrAuthFailed
}

func (authFailingExecutor) Restore(context.Context, coretenant.Scope, domain.PPPSecret) error {
	return mikrotik.ErrAuthFailed
}

func (authFailingExecutor) CurrentProfile(context.Context, coretenant.Scope, domain.PPPSecret) (string, error) {
	return "", mikrotik.ErrAuthFailed
}

type countingAlerter struct{ n int }

func (a *countingAlerter) NotifyTaskFailed(context.Context, coretenant.Scope, service.TaskFailureEvent) error {
	a.n++
	return nil
}

type readyRouter struct{}

func (readyRouter) Get(_ context.Context, _ coretenant.Scope, id string) (domain.Router, error) {
	return domain.Router{ID: id, Name: "R1", IsolirReady: true, IsolirProfileName: "isolir"}, nil
}

func TestWorker_PermanentFailure_NotReenqueuedEveryTick(t *testing.T) {
	f := newIsoFixture(t)
	e := f.e
	secret, err := secretRepo(e).FindByID(e.ctx, f.scope, f.secretID)
	if err != nil || secret.AccountID == nil {
		t.Fatal(err, secret)
	}
	src := oneOverdue{account: *secret.AccountID}
	tasks, secrets := taskRepo(e), secretRepo(e)
	ev := service.NewEvaluator(service.EvaluatorDeps{
		Source: src, Policy: repository.NewPolicyRepository(e.db), Secrets: secrets, Tasks: tasks, Mode: service.ModeOn,
	})
	alert := &countingAlerter{}
	proc := service.NewTaskProcessor(service.TaskProcessorDeps{
		Tasks: tasks, Secrets: secrets, Routers: readyRouter{}, Evaluator: ev,
		Executor: authFailingExecutor{}, Locker: repository.NewAdvisoryLocker(e.db), Alert: alert,
	})
	rec := service.NewReconciler(src, secrets, ev, nil)

	for tick := 0; tick < 5; tick++ {
		if _, err := rec.RunOrganization(e.ctx, f.scope); err != nil {
			t.Fatalf("tick %d reconcile: %v", tick, err)
		}
		if _, err := proc.RunOnce(e.ctx, f.scope, 10); err != nil {
			t.Fatalf("tick %d process: %v", tick, err)
		}
	}
	items := f.pending(t)
	if len(items) != 1 || items[0].Status != domain.TaskFailed {
		t.Fatalf("tasks after 5 ticks = %d (%+v), want 1 failed", len(items), items)
	}
	if alert.n != 1 {
		t.Fatalf("alerts = %d, want 1", alert.n)
	}
}
