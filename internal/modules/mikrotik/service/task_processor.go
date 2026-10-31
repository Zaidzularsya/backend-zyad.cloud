package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

const (
	// taskLease bounds how long a claimed task stays invisible to other
	// workers; a crashed worker's task is reclaimed after it expires.
	taskLease = 2 * time.Minute
	// routerBusyDelay is how long a task waits when another worker holds its
	// router; it does not count as an attempt.
	routerBusyDelay = 15 * time.Second

	eventTaskCompleted = "task_completed"

	cancelSuperseded   = "superseded"
	cancelStateChanged = "state changed"
)

// TaskExecutor performs the router-side steps; satisfied by *Executor.
type TaskExecutor interface {
	Isolate(ctx context.Context, scope coretenant.Scope, secret domain.PPPSecret) (string, error)
	Restore(ctx context.Context, scope coretenant.Scope, secret domain.PPPSecret) error
}

type TaskProcessorDeps struct {
	Tasks     repository.TaskRepository
	Secrets   repository.SecretRepository
	Routers   RouterReader
	Evaluator *Evaluator
	Executor  TaskExecutor
	Locker    RouterLocker
	Customer  CustomerNotifier // optional
	Alert     TenantAlerter    // optional
	Audit     Auditor          // optional
	Now       func() time.Time
}

// ProcessResult counts the outcome of each claimed task in one RunOnce.
type ProcessResult struct{ Done, Retried, Failed, Cancelled, Skipped int }

type outcome int

const (
	outcomeNone outcome = iota // panicked: the task keeps its lease
	outcomeDone
	outcomeRetried
	outcomeFailed
	outcomeCancelled
	outcomeSkipped
)

// TaskProcessor executes claimed isolate/restore tasks against routers.
// Router safety rests on the per-router advisory lock (the task repository has
// no lease-owner check) and on re-evaluating each task before touching the router.
type TaskProcessor struct{ d TaskProcessorDeps }

func NewTaskProcessor(d TaskProcessorDeps) *TaskProcessor {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &TaskProcessor{d: d}
}

// RunOnce claims up to batch due tasks and processes them one by one. A
// failing (or panicking) task never stops the batch; an error is returned only
// when claiming fails or ctx is cancelled.
func (p *TaskProcessor) RunOnce(ctx context.Context, scope coretenant.Scope, batch int) (ProcessResult, error) {
	var res ProcessResult
	if err := ctx.Err(); err != nil {
		return res, err
	}
	tasks, err := p.d.Tasks.ClaimBatch(ctx, scope, batch, p.d.Now().UTC(), taskLease)
	if err != nil {
		return res, fmt.Errorf("claim isolation tasks: %w", err)
	}
	for _, task := range tasks {
		// Unprocessed claimed tasks are reclaimed when their lease expires.
		if err := ctx.Err(); err != nil {
			return res, err
		}
		switch p.processSafely(ctx, scope, task) {
		case outcomeDone:
			res.Done++
		case outcomeRetried:
			res.Retried++
		case outcomeFailed:
			res.Failed++
		case outcomeCancelled:
			res.Cancelled++
		case outcomeSkipped:
			res.Skipped++
		}
	}
	return res, nil
}

func (p *TaskProcessor) processSafely(ctx context.Context, scope coretenant.Scope, task domain.IsolationTask) (out outcome) {
	defer func() {
		if r := recover(); r != nil {
			// The panic value is not logged: it could carry arbitrary data.
			slog.Error("mikrotik: isolation task panicked; left for lease expiry",
				"task_id", task.ID, "secret_id", task.SecretID, "action", task.Action,
				"panic_type", fmt.Sprintf("%T", r), "stack", string(debug.Stack()))
			out = outcomeNone
		}
	}()
	return p.process(ctx, scope, task)
}

func (p *TaskProcessor) process(ctx context.Context, scope coretenant.Scope, task domain.IsolationTask) outcome {
	secret, err := p.d.Secrets.FindByID(ctx, scope, task.SecretID)
	if errors.Is(err, mikrotik.ErrSecretNotFound) {
		return p.cancel(ctx, scope, task, "", "", cancelSuperseded)
	}
	if err != nil {
		return p.release(ctx, scope, task, "load secret", err)
	}

	unlock, ok, err := p.d.Locker.TryLock(ctx, "mikrotik:router:"+secret.RouterID)
	if err != nil {
		return p.release(ctx, scope, task, "lock router", err)
	}
	if !ok {
		return p.release(ctx, scope, task, "", nil)
	}
	defer unlock()

	// Re-read under the lock: the row may have changed while we waited.
	secret, err = p.d.Secrets.FindByID(ctx, scope, task.SecretID)
	if errors.Is(err, mikrotik.ErrSecretNotFound) {
		return p.cancel(ctx, scope, task, "", "", cancelSuperseded)
	}
	if err != nil {
		return p.release(ctx, scope, task, "reload secret", err)
	}
	router, err := p.d.Routers.Get(ctx, scope, secret.RouterID)
	routerMissing := errors.Is(err, mikrotik.ErrRouterNotFound)
	if err != nil && !routerMissing {
		return p.release(ctx, scope, task, "load router", err)
	}

	want, dec, err := p.wantedAction(ctx, scope, secret)
	if err != nil {
		return p.release(ctx, scope, task, "re-evaluate", err)
	}
	// Isolating needs a live, isolir-ready router. A restore never waits on
	// isolir_ready; a restore on a deleted router fails and alerts instead.
	if want == ActionIsolate && (routerMissing || !router.IsolirReady) {
		want = ""
	}
	accountID := ""
	if secret.AccountID != nil {
		accountID = *secret.AccountID
	}
	if want != task.Action {
		return p.cancel(ctx, scope, task, secret.RouterID, accountID, cancelSuperseded)
	}

	switch task.Action {
	case ActionIsolate:
		original, err := p.d.Executor.Isolate(ctx, scope, secret)
		if err != nil {
			return p.failure(ctx, scope, task, secret, router.Name, err)
		}
		changed, err := p.d.Secrets.MarkIsolated(ctx, scope, secret.ID, original)
		return p.succeeded(ctx, scope, task, secret, router.Name, accountID, dec, changed, err)
	default: // ActionRestore; any other action was cancelled above.
		if err := p.d.Executor.Restore(ctx, scope, secret); err != nil {
			return p.failure(ctx, scope, task, secret, router.Name, err)
		}
		changed, err := p.d.Secrets.MarkNormal(ctx, scope, secret.ID)
		return p.succeeded(ctx, scope, task, secret, router.Name, accountID, dec, changed, err)
	}
}

// wantedAction is the action the secret needs right now. An unpaired secret is
// never isolated; an unpaired isolated secret (should not happen: unpair
// refuses isolated secrets) is still restored rather than left isolated.
func (p *TaskProcessor) wantedAction(ctx context.Context, scope coretenant.Scope, secret domain.PPPSecret) (string, Decision, error) {
	if secret.AccountID == nil {
		if secret.IsolationState == domain.IsolationIsolated {
			return ActionRestore, Decision{}, nil
		}
		return "", Decision{}, nil
	}
	dec, err := p.d.Evaluator.Decide(ctx, scope, *secret.AccountID)
	if err != nil {
		return "", Decision{}, err
	}
	for _, a := range dec.Secrets {
		if a.SecretID == secret.ID {
			return a.Action, dec, nil
		}
	}
	return "", dec, nil
}

// succeeded records a confirmed router change. The secret state follows the
// router even when the task itself was cancelled or finished meanwhile.
func (p *TaskProcessor) succeeded(ctx context.Context, scope coretenant.Scope, task domain.IsolationTask, secret domain.PPPSecret,
	routerName, accountID string, dec Decision, changed bool, markErr error) outcome {
	if markErr != nil {
		// The router is changed but the DB is not: retry; the router steps are
		// idempotent, so the next attempt only confirms and flips the state.
		return p.failure(ctx, scope, task, secret, routerName, fmt.Errorf("mark secret state: %w", markErr))
	}
	if !changed {
		return p.cancel(ctx, scope, task, secret.RouterID, accountID, cancelStateChanged)
	}
	if err := p.d.Tasks.Complete(ctx, scope, task.ID); err != nil {
		p.logLifecycleErr("complete", task, err)
	}
	event := "isolation.isolated"
	if task.Action == ActionRestore {
		event = "isolation.restored"
	}
	p.audit(ctx, scope, event, secret.RouterID, task, task.Attempts+1)
	p.notifyCustomer(ctx, scope, task, secret.RouterID, accountID, dec)
	p.followUp(ctx, scope, accountID)
	return outcomeDone
}

func (p *TaskProcessor) failure(ctx context.Context, scope coretenant.Scope, task domain.IsolationTask, secret domain.PPPSecret, routerName string, cause error) outcome {
	f := Classify(cause)
	attempts := task.Attempts + 1
	slog.Warn("mikrotik: isolation task attempt failed",
		"task_id", task.ID, "secret_id", task.SecretID, "action", task.Action, "attempts", attempts, "code", f.Code, "error", cause)
	if errors.Is(cause, mikrotik.ErrSecretMissingOnRouter) {
		if err := p.d.Secrets.MarkAbsent(ctx, scope, secret.ID); err != nil {
			slog.Error("mikrotik: mark secret absent failed", "secret_id", secret.ID, "error", err)
		}
	}
	event := TaskFailureEvent{TaskID: task.ID, RouterName: routerName, SecretName: secret.Name, Action: task.Action, Reason: f.Message}

	next, exhausted := NextAttempt(task.Action, attempts, p.d.Now().UTC())
	if f.Kind == FailPermanent || exhausted {
		// No follow-up evaluation here: it could only re-enqueue the same
		// failing action and loop every tick.
		if err := p.d.Tasks.Fail(ctx, scope, task.ID, f.Code, f.Message); err != nil {
			p.logLifecycleErr("fail", task, err)
			return outcomeFailed
		}
		p.audit(ctx, scope, "task.failed", secret.RouterID, task, attempts)
		p.alertTenant(ctx, scope, task, event)
		return outcomeFailed
	}
	if err := p.d.Tasks.RetryLater(ctx, scope, task.ID, next, f.Code, f.Message); err != nil {
		p.logLifecycleErr("retry later", task, err)
		return outcomeRetried
	}
	if ShouldNotifyTenant(task.Action, attempts) {
		p.alertTenant(ctx, scope, task, event)
	}
	return outcomeRetried
}

// cancel ends a task without touching the router and re-evaluates the account
// so the action that is now needed (if any) is enqueued.
func (p *TaskProcessor) cancel(ctx context.Context, scope coretenant.Scope, task domain.IsolationTask, routerID, accountID, reason string) outcome {
	slog.Info("mikrotik: isolation task cancelled", "task_id", task.ID, "secret_id", task.SecretID, "action", task.Action, "reason", reason)
	if err := p.d.Tasks.Cancel(ctx, scope, task.ID, reason); err != nil {
		p.logLifecycleErr("cancel", task, err)
	} else {
		p.audit(ctx, scope, "task.cancelled", routerID, task, task.Attempts)
	}
	p.followUp(ctx, scope, accountID)
	return outcomeCancelled
}

// release returns the task to pending without counting an attempt. cause is
// nil when the router is simply busy.
func (p *TaskProcessor) release(ctx context.Context, scope coretenant.Scope, task domain.IsolationTask, step string, cause error) outcome {
	if cause != nil {
		slog.Error("mikrotik: isolation task deferred", "task_id", task.ID, "step", step, "error", cause)
	}
	if err := p.d.Tasks.Release(ctx, scope, task.ID, p.d.Now().UTC().Add(routerBusyDelay)); err != nil {
		p.logLifecycleErr("release", task, err)
	}
	return outcomeSkipped
}

// followUp re-evaluates the account after a task ended. Needed because an
// enqueue refused while this task was active (e.g. a payment during a running
// isolate) is not retried by anyone else until the reconciler runs.
func (p *TaskProcessor) followUp(ctx context.Context, scope coretenant.Scope, accountID string) {
	if accountID == "" || p.d.Evaluator == nil {
		return
	}
	if _, err := p.d.Evaluator.Apply(ctx, scope, accountID, eventTaskCompleted); err != nil {
		slog.Error("mikrotik: follow-up evaluation failed", "account_id", accountID, "error", err)
	}
}

func (p *TaskProcessor) notifyCustomer(ctx context.Context, scope coretenant.Scope, task domain.IsolationTask, routerID, accountID string, dec Decision) {
	if p.d.Customer == nil || accountID == "" || task.CustomerNotifiedAt != nil {
		return
	}
	var err error
	if task.Action == ActionIsolate {
		err = p.d.Customer.NotifyIsolated(ctx, scope, accountID, dec.Invoices)
	} else {
		err = p.d.Customer.NotifyRestored(ctx, scope, accountID)
	}
	if err != nil {
		slog.Warn("mikrotik: customer notification failed", "task_id", task.ID, "action", task.Action, "error", err)
		p.audit(ctx, scope, "notification.failed", routerID, task, task.Attempts+1)
	}
	// Marked even after a failure so a retried task does not resend.
	if err := p.d.Tasks.MarkCustomerNotified(ctx, scope, task.ID); err != nil {
		slog.Error("mikrotik: mark customer notified failed", "task_id", task.ID, "error", err)
	}
}

// alertTenant sends at most one failure alert per task.
func (p *TaskProcessor) alertTenant(ctx context.Context, scope coretenant.Scope, task domain.IsolationTask, e TaskFailureEvent) {
	if p.d.Alert == nil || task.TenantNotifiedAt != nil {
		return
	}
	if err := p.d.Alert.NotifyTaskFailed(ctx, scope, e); err != nil {
		slog.Warn("mikrotik: tenant alert failed", "task_id", task.ID, "error", err)
	}
	if err := p.d.Tasks.MarkTenantNotified(ctx, scope, task.ID); err != nil {
		slog.Error("mikrotik: mark tenant notified failed", "task_id", task.ID, "error", err)
	}
}

func (p *TaskProcessor) audit(ctx context.Context, scope coretenant.Scope, event, routerID string, task domain.IsolationTask, attempts int) {
	if p.d.Audit == nil {
		return
	}
	err := p.d.Audit.InsertAudit(ctx, scope, repository.AuditEntry{
		Event:    event,
		TargetID: routerID,
		Metadata: map[string]any{
			"target": "task", "task_id": task.ID, "secret_id": task.SecretID,
			"action": task.Action, "attempts": attempts,
		},
	})
	if err != nil {
		slog.Error("mikrotik: audit failed", "event", event, "task_id", task.ID, "error", err)
	}
}

// logLifecycleErr: ErrTaskNotFound means the task was cancelled or finished by
// someone else meanwhile, which is a normal outcome.
func (p *TaskProcessor) logLifecycleErr(step string, task domain.IsolationTask, err error) {
	if errors.Is(err, mikrotik.ErrTaskNotFound) {
		slog.Info("mikrotik: isolation task no longer active", "task_id", task.ID, "step", step)
		return
	}
	slog.Error("mikrotik: isolation task update failed", "task_id", task.ID, "step", step, "error", err)
}
