package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

// Isolation engine modes (config.MikrotikConfig.IsolationMode). Any value other
// than "on" or "dry_run" is treated as off.
const (
	ModeOff    = "off"
	ModeDryRun = "dry_run"
	ModeOn     = "on"
)

const (
	actionIsolate = "isolate"
	actionRestore = "restore"
)

// SecretAction is the action needed on one PPP secret; Action is "" when the
// secret already matches the desired state (or is not considered).
type SecretAction struct{ SecretID, Action string }

// Decision is the evaluated isolation state of one customer account.
// IsolateAllowed/BlockedReason come from IsolateAllowed (env mode, tenant
// control, pilot membership); they only ever gate isolate actions.
type Decision struct {
	AccountID      string
	Desired        string // domain.IsolationNormal | domain.IsolationIsolated
	Exempt         bool   // effective exemption (expired ones do not count)
	Invoices       []TriggerInvoice
	Secrets        []SecretAction
	IsolateAllowed bool
	BlockedReason  string
	// IsPilot: the account is in the tenant's pilot list (the tenant's own
	// test account); customer notifications are never sent for it.
	IsPilot bool
}

// ControlReader reads the tenant isolation control; satisfied by
// repository.ControlRepository.
type ControlReader interface {
	Get(ctx context.Context, scope coretenant.Scope) (domain.ControlSettings, error)
	PilotState(ctx context.Context, scope coretenant.Scope, accountID string) (isPilot, testActive bool, err error)
}

type EvaluatorDeps struct {
	Source  OverdueSource
	Policy  repository.PolicyRepository
	Secrets repository.SecretRepository
	Tasks   repository.TaskRepository
	Now     func() time.Time
	Mode    string
	// Control is optional: nil behaves as tenant mode active, grace 0 and
	// no pilot accounts (the Gelombang 1 behavior).
	Control ControlReader
}

// Evaluator derives the desired isolation state of a customer account and
// enqueues the isolate/restore tasks needed to reach it.
type Evaluator struct{ d EvaluatorDeps }

func NewEvaluator(d EvaluatorDeps) *Evaluator {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Evaluator{d: d}
}

func (e *Evaluator) mode() string {
	switch e.d.Mode {
	case ModeOn, ModeDryRun:
		return e.d.Mode
	}
	return ModeOff
}

// control loads the tenant control snapshot used for one evaluation (or one
// reconcile run). A failed read never fails the evaluation: it yields a
// snapshot that blocks every isolation (ReasonControlUnreadable) with grace 0,
// so restores are still evaluated and enqueued.
func (e *Evaluator) control(ctx context.Context, scope coretenant.Scope) domain.ControlSettings {
	if e.d.Control == nil {
		return domain.ControlSettings{OrganizationID: scope.OrganizationID(), Mode: domain.ModeActive}
	}
	ctl, err := e.d.Control.Get(ctx, scope)
	if err != nil {
		slog.ErrorContext(ctx, "mikrotik: isolation control unreadable; isolation blocked, restores continue",
			"organization_id", scope.OrganizationID(), "error", scrub(err.Error()))
		return domain.ControlSettings{OrganizationID: scope.OrganizationID(), Mode: modeControlUnreadable}
	}
	return ctl
}

// pilotState treats a failed read as "not a pilot, no test running": that can
// only block an isolation (pilot mode) or end a pilot test, never a restore.
func (e *Evaluator) pilotState(ctx context.Context, scope coretenant.Scope, accountID string) (bool, bool) {
	if e.d.Control == nil {
		return false, false
	}
	isPilot, testActive, err := e.d.Control.PilotState(ctx, scope, accountID)
	if err != nil {
		slog.ErrorContext(ctx, "mikrotik: pilot state unreadable; treated as not a pilot",
			"account_id", accountID, "error", scrub(err.Error()))
		return false, false
	}
	return isPilot, testActive
}

// isolateGate is the isolation gate of one account without evaluating its
// invoices or secrets (used before taking a router lock).
func (e *Evaluator) isolateGate(ctx context.Context, scope coretenant.Scope, accountID string) GateResult {
	isPilot, _ := e.pilotState(ctx, scope, accountID)
	return IsolateAllowed(e.mode(), e.control(ctx, scope), isPilot)
}

// Decide is read-only: it never writes tasks or secrets.
func (e *Evaluator) Decide(ctx context.Context, scope coretenant.Scope, accountID string) (Decision, error) {
	return e.decide(ctx, scope, accountID, e.control(ctx, scope))
}

// decide evaluates the account against one control snapshot. An account is
// isolated when it has a trigger invoice past the grace cutoff, or when a
// pilot test is running on it, unless it is effectively exempt.
func (e *Evaluator) decide(ctx context.Context, scope coretenant.Scope, accountID string, ctl domain.ControlSettings) (Decision, error) {
	today := businesstime.DayOf(e.d.Now())
	invoices, err := e.d.Source.TriggerInvoices(ctx, scope, accountID, CandidateCutoff(today, ctl.GraceDays))
	if err != nil {
		return Decision{}, fmt.Errorf("load trigger invoices: %w", err)
	}
	policy, _, err := e.d.Policy.GetCustomerPolicy(ctx, scope, accountID)
	if err != nil {
		return Decision{}, fmt.Errorf("load customer policy: %w", err)
	}
	secrets, err := e.d.Secrets.ListByAccount(ctx, scope, accountID)
	if err != nil {
		return Decision{}, fmt.Errorf("list account secrets: %w", err)
	}
	isPilot, testActive := e.pilotState(ctx, scope, accountID)

	exempt := EffectiveExempt(policy, today)
	gate := IsolateAllowed(e.mode(), ctl, isPilot)
	dec := Decision{
		AccountID: accountID, Desired: domain.IsolationNormal, Exempt: exempt, Invoices: invoices,
		IsolateAllowed: gate.Allowed, BlockedReason: gate.Reason, IsPilot: isPilot,
	}
	if (len(invoices) > 0 || testActive) && !exempt {
		dec.Desired = domain.IsolationIsolated
	}
	for _, s := range secrets {
		act := SecretAction{SecretID: s.ID}
		switch {
		case dec.Desired == domain.IsolationIsolated && s.IsolationState == domain.IsolationNormal && s.Present && !s.NeedsOriginalProfile:
			act.Action = actionIsolate
		case dec.Desired == domain.IsolationNormal && s.IsolationState == domain.IsolationIsolated:
			// Restore regardless of presence: the secret must not stay isolated.
			act.Action = actionRestore
		}
		dec.Secrets = append(dec.Secrets, act)
	}
	return dec, nil
}

// Apply evaluates the account and, in mode "on", enqueues one task per secret
// that needs an action. A failure on one secret does not stop the others.
// enqueued=false from the repository (not eligible / active task) is not an error.
func (e *Evaluator) Apply(ctx context.Context, scope coretenant.Scope, accountID, event string) (Decision, error) {
	return e.apply(ctx, scope, accountID, event, false)
}

// ApplyRestoreOnly is Apply for an organization that lost the mikrotik.enabled
// entitlement: it may enqueue restore tasks but never an isolate task (the
// isolate actions are dropped from the returned decision). Modes behave as in
// Apply: off is a no-op, dry_run only logs.
func (e *Evaluator) ApplyRestoreOnly(ctx context.Context, scope coretenant.Scope, accountID, event string) (Decision, error) {
	return e.apply(ctx, scope, accountID, event, true)
}

func (e *Evaluator) apply(ctx context.Context, scope coretenant.Scope, accountID, event string, restoreOnly bool) (Decision, error) {
	if e.mode() == ModeOff {
		return Decision{}, nil
	}
	return e.applyWith(ctx, scope, accountID, event, restoreOnly, e.control(ctx, scope))
}

// applyWith is apply against a control snapshot read by the caller (the
// reconciler reads it once per run so every account sees the same settings).
// Restores are never gated by the tenant mode; isolates are dropped when
// restoreOnly or when the gate blocks them.
func (e *Evaluator) applyWith(ctx context.Context, scope coretenant.Scope, accountID, event string, restoreOnly bool, ctl domain.ControlSettings) (Decision, error) {
	mode := e.mode()
	if mode == ModeOff {
		return Decision{}, nil
	}
	dec, err := e.decide(ctx, scope, accountID, ctl)
	if err != nil {
		return Decision{}, err
	}
	if restoreOnly {
		dropIsolates(&dec)
	}
	if mode == ModeDryRun {
		logDecision(ctx, "mikrotik: dry-run decision", dec, event, restoreOnly)
		return dec, nil
	}
	if ctl.Mode == domain.ModeSimulation {
		// Logged before the gate drops the isolates: the would-be actions
		// are what a simulation is for.
		logDecision(ctx, "mikrotik: simulation decision", dec, event, restoreOnly)
	}
	if !dec.IsolateAllowed {
		dropIsolates(&dec)
	}
	var errs []error
	for _, a := range dec.Secrets {
		var qErr error
		switch a.Action {
		case actionIsolate:
			_, qErr = e.d.Tasks.EnqueueIsolate(ctx, scope, a.SecretID, event)
		case actionRestore:
			_, qErr = e.d.Tasks.EnqueueRestore(ctx, scope, a.SecretID, event)
		default:
			continue
		}
		if qErr != nil {
			slog.Error("mikrotik: enqueue isolation task failed", "account_id", accountID, "secret_id", a.SecretID, "action", a.Action, "error", qErr)
			errs = append(errs, fmt.Errorf("secret %s %s: %w", a.SecretID, a.Action, qErr))
		}
	}
	return dec, errors.Join(errs...)
}

func dropIsolates(dec *Decision) {
	for i := range dec.Secrets {
		if dec.Secrets[i].Action == actionIsolate {
			dec.Secrets[i].Action = ""
		}
	}
}

// logDecision logs a decision that is not executed. Info only when something
// would happen; a no-op decision is logged at Debug to keep the periodic
// reconcile quiet.
func logDecision(ctx context.Context, msg string, dec Decision, event string, restoreOnly bool) {
	var ids []string
	for _, a := range dec.Secrets {
		if a.Action != "" {
			ids = append(ids, a.SecretID+":"+a.Action)
		}
	}
	level := slog.LevelDebug
	if len(ids) > 0 {
		level = slog.LevelInfo
	}
	slog.Log(ctx, level, msg,
		"account_id", dec.AccountID, "desired", dec.Desired, "exempt", dec.Exempt,
		"invoices", len(dec.Invoices), "secret_actions", ids, "event", event, "restore_only", restoreOnly)
}
