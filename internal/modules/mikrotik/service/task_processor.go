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

	eventTaskCompleted     = "task_completed"
	eventTaskFailedPartial = "task_failed_partial"

	cancelSuperseded   = "superseded"
	cancelStateChanged = "state changed"
	// cancelEntitlementDisabled: an isolate task claimed while the
	// organization has no mikrotik.enabled entitlement (restore-only mode).
	cancelEntitlementDisabled = "entitlement_disabled"
)

// TaskExecutor performs the router-side steps; satisfied by *Executor.
type TaskExecutor interface {
	Isolate(ctx context.Context, scope coretenant.Scope, secret domain.PPPSecret) (string, error)
	Restore(ctx context.Context, scope coretenant.Scope, secret domain.PPPSecret) error
	// CurrentProfile reads (never writes) the secret's profile on the router.
	CurrentProfile(ctx context.Context, scope coretenant.Scope, secret domain.PPPSecret) (string, error)
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
type TaskProcessor struct {
	d TaskProcessorDeps
	// restoreOnly is set only on the per-call copy made by RunRestoreOnly.
	restoreOnly bool
}

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
	return p.runOnce(ctx, scope, batch)
}

// RunRestoreOnly is RunOnce for an organization that lost the mikrotik.enabled
// entitlement: restore tasks run normally, every claimed isolate task is
// cancelled ("entitlement_disabled") without touching the router, and
// follow-up evaluations never enqueue an isolate.
func (p *TaskProcessor) RunRestoreOnly(ctx context.Context, scope coretenant.Scope, batch int) (ProcessResult, error) {
	ro := &TaskProcessor{d: p.d, restoreOnly: true}
	return ro.runOnce(ctx, scope, batch)
}

func (p *TaskProcessor) runOnce(ctx context.Context, scope coretenant.Scope, batch int) (ProcessResult, error) {
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
	restoreOnlyIsolate := p.restoreOnly && task.Action == ActionIsolate
	if restoreOnlyIsolate && !taskMayHaveRun(task) {
		accountID := ""
		if secret.AccountID != nil {
			accountID = *secret.AccountID
		}
		return p.cancel(ctx, scope, task, secret.RouterID, accountID, cancelEntitlementDisabled)
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

	accountID := ""
	if secret.AccountID != nil {
		accountID = *secret.AccountID
	}
	if restoreOnlyIsolate {
		return p.cancelIsolate(ctx, scope, task, secret, router, routerMissing, accountID, cancelEntitlementDisabled)
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
	if want != task.Action {
		if task.Action == ActionIsolate {
			return p.cancelIsolate(ctx, scope, task, secret, router, routerMissing, accountID, cancelSuperseded)
		}
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
		} else {
			p.audit(ctx, scope, "task.failed", secret.RouterID, task, attempts)
			p.alertTenant(ctx, scope, task, event)
		}
		// Independent of Fail: the router may already be changed.
		p.alignPartialIsolate(ctx, scope, task, secret, cause, attempts)
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

// alignPartialIsolate handles an isolate that failed for good after the router
// was already on the isolir profile. Without it the DB says "normal", so a
// customer who pays never gets a restore. Transient failures are left to the
// retry, which finishes the job idempotently.
func (p *TaskProcessor) alignPartialIsolate(ctx context.Context, scope coretenant.Scope, task domain.IsolationTask, secret domain.PPPSecret, cause error, attempts int) {
	var pe *ProfileChangedError
	if task.Action != ActionIsolate || !errors.As(cause, &pe) {
		return
	}
	changed, err := p.d.Secrets.MarkIsolated(ctx, scope, secret.ID, pe.Original)
	if err != nil {
		slog.Error("mikrotik: mark partially isolated secret failed", "task_id", task.ID, "secret_id", secret.ID, "error", err)
		return
	}
	if !changed {
		return
	}
	p.audit(ctx, scope, "isolation.isolated", secret.RouterID, task, attempts, "partial", true)
	if secret.AccountID != nil && p.d.Evaluator != nil {
		if _, err := p.d.Evaluator.Apply(ctx, scope, *secret.AccountID, eventTaskFailedPartial); err != nil {
			slog.Error("mikrotik: follow-up evaluation failed", "account_id", *secret.AccountID, "error", err)
		}
	}
}

// taskMayHaveRun reports whether an earlier attempt (or a worker that crashed
// mid-task) may already have put the isolir profile on the router. ClaimBatch
// sets StartedAt on every claim, so in practice every claimed task qualifies:
// a reclaimed crashed task is indistinguishable from a fresh one.
func taskMayHaveRun(task domain.IsolationTask) bool {
	return task.Attempts > 0 || task.StartedAt != nil
}

// cancelIsolate cancels an isolate task that is no longer wanted. The caller
// holds the router lock. If the task may already have run, the router is read
// first: an earlier attempt that applied the isolir profile and then failed
// transiently (or a crash between Set and MarkIsolated) leaves the router
// isolated while the DB says normal, and nothing would ever restore it. The DB
// is aligned to the router so the follow-up evaluation in cancel enqueues the
// restore. A transient read failure releases the task instead of cancelling.
func (p *TaskProcessor) cancelIsolate(ctx context.Context, scope coretenant.Scope, task domain.IsolationTask, secret domain.PPPSecret,
	router domain.Router, routerMissing bool, accountID, reason string) outcome {
	if !taskMayHaveRun(task) || routerMissing || router.IsolirProfileName == "" || secret.IsolationState != domain.IsolationNormal {
		return p.cancel(ctx, scope, task, secret.RouterID, accountID, reason)
	}
	current, err := p.d.Executor.CurrentProfile(ctx, scope, secret)
	switch {
	case err == nil:
	case errors.Is(err, mikrotik.ErrSecretMissingOnRouter):
		// Nothing on the router to align.
		return p.cancel(ctx, scope, task, secret.RouterID, accountID, reason)
	case Classify(err).Kind == FailTransient:
		return p.release(ctx, scope, task, "read router profile before cancel", err)
	default:
		// Permanent (auth, TLS, ...): retrying the read cannot help. Cancel as
		// before; a human has to fix the router access first.
		slog.Warn("mikrotik: cannot read router profile before cancelling isolate",
			"task_id", task.ID, "secret_id", secret.ID, "code", Classify(err).Code)
		return p.cancel(ctx, scope, task, secret.RouterID, accountID, reason)
	}
	if current != router.IsolirProfileName {
		return p.cancel(ctx, scope, task, secret.RouterID, accountID, reason)
	}
	// "" keeps the stored original profile (isolate eligibility requires one).
	changed, err := p.d.Secrets.MarkIsolated(ctx, scope, secret.ID, "")
	if err != nil {
		return p.release(ctx, scope, task, "mark partially isolated secret", err)
	}
	if changed {
		p.audit(ctx, scope, "isolation.isolated", secret.RouterID, task, task.Attempts, "partial", true)
	} else {
		// Only an unpaired secret is refused here, and unpair is refused while a
		// task is active; log loudly if it ever happens.
		slog.Error("mikrotik: router is on the isolir profile but the secret could not be marked isolated",
			"task_id", task.ID, "secret_id", secret.ID)
	}
	return p.cancel(ctx, scope, task, secret.RouterID, accountID, reason)
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
	apply := p.d.Evaluator.Apply
	if p.restoreOnly {
		apply = p.d.Evaluator.ApplyRestoreOnly
	}
	if _, err := apply(ctx, scope, accountID, eventTaskCompleted); err != nil {
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

// audit writes a system audit entry; extra is key/value pairs added to the
// standard task metadata.
func (p *TaskProcessor) audit(ctx context.Context, scope coretenant.Scope, event, routerID string, task domain.IsolationTask, attempts int, extra ...any) {
	if p.d.Audit == nil {
		return
	}
	meta := map[string]any{
		"target": "task", "task_id": task.ID, "secret_id": task.SecretID,
		"action": task.Action, "attempts": attempts,
	}
	for i := 0; i+1 < len(extra); i += 2 {
		if k, ok := extra[i].(string); ok {
			meta[k] = extra[i+1]
		}
	}
	err := p.d.Audit.InsertAudit(ctx, scope, repository.AuditEntry{Event: event, TargetID: routerID, Metadata: meta})
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
