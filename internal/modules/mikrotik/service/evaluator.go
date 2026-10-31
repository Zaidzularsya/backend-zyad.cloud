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
type Decision struct {
	AccountID string
	Desired   string // domain.IsolationNormal | domain.IsolationIsolated
	Exempt    bool
	Invoices  []TriggerInvoice
	Secrets   []SecretAction
}

type EvaluatorDeps struct {
	Source  OverdueSource
	Policy  repository.PolicyRepository
	Secrets repository.SecretRepository
	Tasks   repository.TaskRepository
	Now     func() time.Time
	Mode    string
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

// Decide is read-only: it never writes tasks or secrets.
func (e *Evaluator) Decide(ctx context.Context, scope coretenant.Scope, accountID string) (Decision, error) {
	today := businesstime.DayOf(e.d.Now())
	invoices, err := e.d.Source.TriggerInvoices(ctx, scope, accountID, today)
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

	dec := Decision{AccountID: accountID, Desired: domain.IsolationNormal, Exempt: policy.Exempt, Invoices: invoices}
	if len(invoices) > 0 && !policy.Exempt {
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
	mode := e.mode()
	if mode == ModeOff {
		return Decision{}, nil
	}
	dec, err := e.Decide(ctx, scope, accountID)
	if err != nil {
		return Decision{}, err
	}
	if mode == ModeDryRun {
		var ids []string
		for _, a := range dec.Secrets {
			if a.Action != "" {
				ids = append(ids, a.SecretID+":"+a.Action)
			}
		}
		slog.Info("mikrotik: dry-run decision",
			"account_id", accountID, "desired", dec.Desired, "exempt", dec.Exempt,
			"invoices", len(dec.Invoices), "secret_actions", ids, "event", event)
		return dec, nil
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
