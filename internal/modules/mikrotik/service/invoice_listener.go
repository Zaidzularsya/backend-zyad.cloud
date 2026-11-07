package service

import (
	"context"
	"errors"
	"log/slog"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
)

// InvoiceListener re-evaluates the owning account when an invoice changes
// (paid, replayed, voided). The receivable adapter that calls it lives in
// internal/app so this module does not import receivable.
type InvoiceListener struct {
	source OverdueSource
	eval   *Evaluator
}

func NewInvoiceListener(source OverdueSource, eval *Evaluator) *InvoiceListener {
	return &InvoiceListener{source: source, eval: eval}
}

// OnInvoiceChanged never returns an error or panics: it runs after the
// invoice change committed and the periodic job is the safety net.
func (l *InvoiceListener) OnInvoiceChanged(ctx context.Context, scope coretenant.Scope, invoiceID, event string) {
	accountID, err := l.source.AccountOfInvoice(ctx, scope, invoiceID)
	if errors.Is(err, mikrotik.ErrAccountNotFound) {
		slog.Debug("mikrotik: invoice has no account in this organization", "invoice_id", invoiceID, "event", event)
		return
	}
	if err != nil {
		slog.Warn("mikrotik: resolve invoice account failed", "invoice_id", invoiceID, "event", event, "error", err)
		return
	}
	if _, err := l.eval.Apply(ctx, scope, accountID, event); err != nil {
		slog.Warn("mikrotik: evaluate account failed", "account_id", accountID, "invoice_id", invoiceID, "event", event, "error", err)
	}
}
