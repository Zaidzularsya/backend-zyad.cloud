package service

import (
	"context"
	"errors"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

// Stage is the recovery progress shown on the public "paid" page.
type Stage string

const (
	StageReceived      Stage = "received"
	StageRestoring     Stage = "restoring"
	StageActive        Stage = "active"
	StageNeedsHelp     Stage = "needs_help"
	StageOtherInvoices Stage = "other_invoices"
)

// NeedsHelpAfter is how long a paid customer waits for recovery before the
// page asks them to contact the tenant.
const NeedsHelpAfter = 10 * time.Minute

// Stage derives the recovery stage of the account owning invoiceID.
// applicable=false (no error) when the account has no secrets or the invoice
// is unknown, so callers cannot probe for invoices.
func (s *PublicService) Stage(ctx context.Context, scope coretenant.Scope, invoiceID string, paidAt, now, today time.Time) (Stage, bool, error) {
	accountID, err := s.d.Overdue.AccountOfInvoice(ctx, scope, invoiceID)
	if errors.Is(err, mikrotik.ErrAccountNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	secrets, err := s.d.Secrets.ListByAccount(ctx, scope, accountID)
	if err != nil {
		return "", false, err
	}
	if len(secrets) == 0 {
		return "", false, nil
	}
	var isolated []string
	for _, sec := range secrets {
		if sec.IsolationState == domain.IsolationIsolated {
			isolated = append(isolated, sec.ID)
		}
	}
	if len(isolated) == 0 {
		return StageActive, true, nil
	}
	// Same candidate rule as the evaluator: an invoice still in its grace
	// period, or any invoice of an exempt account, does not keep it isolated.
	remaining, err := s.candidates(ctx, scope, accountID, today)
	if err != nil {
		return "", false, err
	}
	if len(remaining) > 0 {
		return StageOtherInvoices, true, nil
	}
	tasks, err := s.d.Tasks.LatestRestoreBySecret(ctx, scope, isolated)
	if err != nil {
		return "", false, err
	}

	var restoring, needsHelp bool
	for _, id := range isolated {
		t, ok := tasks[id]
		switch {
		case !ok:
			// No restore task: engine mode off or not enqueued yet.
			if now.Sub(paidAt) >= NeedsHelpAfter {
				needsHelp = true
			}
		case t.Status == domain.TaskFailed:
			needsHelp = true
		case t.Status == domain.TaskPending || t.Status == domain.TaskRunning:
			if now.Sub(t.CreatedAt) >= NeedsHelpAfter {
				needsHelp = true
			} else {
				restoring = true
			}
		default:
			// done/cancelled yet still isolated: nothing is in flight.
			if now.Sub(paidAt) >= NeedsHelpAfter {
				needsHelp = true
			}
		}
	}
	switch {
	case needsHelp:
		return StageNeedsHelp, true, nil
	case restoring:
		return StageRestoring, true, nil
	}
	return StageReceived, true, nil
}
