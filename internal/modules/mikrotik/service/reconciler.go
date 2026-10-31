package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

// reconcilePageSize is the keyset page size for AccountsWithTriggerInvoices.
const reconcilePageSize = 200

// eventReconcile is the created_by_event of tasks enqueued by the periodic job.
const eventReconcile = "reconcile"

// ReconcileResult counts the accounts one RunOrganization evaluated.
type ReconcileResult struct {
	Accounts int // distinct accounts evaluated
	Failed   int // accounts whose evaluation/enqueue failed (logged, run continued)
}

// Reconciler is the periodic safety net of the isolation engine. It
// re-evaluates every account that either has a trigger invoice or still has
// an isolated secret.
//
// It is deliberately the ONLY path that:
//   - retries after a task ended in a normal failure (status failed): the
//     task processor never re-enqueues a failed task, so the next reconcile
//     pass evaluates the account again and enqueues a fresh task;
//   - applies product trigger changes (enabling/disabling a product trigger
//     or changing effective_from fires no per-account hook) and plain due
//     dates passing (no event exists for "an invoice became overdue").
//
// Event hooks (payment, void, pairing, exemption) only make reactions faster.
type Reconciler struct {
	source  OverdueSource
	secrets repository.SecretRepository
	eval    *Evaluator
	now     func() time.Time
}

func NewReconciler(source OverdueSource, secrets repository.SecretRepository, eval *Evaluator, now func() time.Time) *Reconciler {
	if now == nil {
		now = time.Now
	}
	return &Reconciler{source: source, secrets: secrets, eval: eval, now: now}
}

// RunOrganization evaluates the union of accounts with isolated secrets and
// accounts with trigger invoices, each exactly once. Isolated accounts go
// first so restores are enqueued even when trigger paging later fails. A
// per-account failure is logged and counted; only listing errors and ctx
// cancellation end the run early (with the partial result).
func (r *Reconciler) RunOrganization(ctx context.Context, scope coretenant.Scope) (ReconcileResult, error) {
	return r.run(ctx, scope, false)
}

// RunOrganizationRestoreOnly is the pass for an organization without the
// mikrotik.enabled entitlement: only accounts with isolated secrets are
// evaluated, through Evaluator.ApplyRestoreOnly, so losing the entitlement can
// restore customers but never isolate anyone.
func (r *Reconciler) RunOrganizationRestoreOnly(ctx context.Context, scope coretenant.Scope) (ReconcileResult, error) {
	return r.run(ctx, scope, true)
}

func (r *Reconciler) run(ctx context.Context, scope coretenant.Scope, restoreOnly bool) (ReconcileResult, error) {
	var res ReconcileResult
	if err := ctx.Err(); err != nil {
		return res, err
	}
	seen := map[string]struct{}{}
	visit := func(accountID string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, ok := seen[accountID]; ok {
			return nil
		}
		seen[accountID] = struct{}{}
		res.Accounts++
		apply := r.eval.Apply
		if restoreOnly {
			apply = r.eval.ApplyRestoreOnly
		}
		if _, err := apply(ctx, scope, accountID, eventReconcile); err != nil {
			res.Failed++
			slog.WarnContext(ctx, "mikrotik: reconcile account failed", "account_id", accountID, "error", err)
		}
		return nil
	}

	isolated, err := r.secrets.AccountsWithIsolatedSecrets(ctx, scope)
	if err != nil {
		return res, fmt.Errorf("list accounts with isolated secrets: %w", err)
	}
	for _, id := range isolated {
		if err := visit(id); err != nil {
			return res, err
		}
	}

	if restoreOnly {
		return res, nil
	}

	today := businesstime.DayOf(r.now())
	after := ""
	for {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		page, err := r.source.AccountsWithTriggerInvoices(ctx, scope, today, after, reconcilePageSize)
		if err != nil {
			return res, fmt.Errorf("list accounts with trigger invoices: %w", err)
		}
		for _, id := range page {
			if err := visit(id); err != nil {
				return res, err
			}
		}
		if len(page) < reconcilePageSize {
			return res, nil
		}
		after = page[len(page)-1]
	}
}
