package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

// ActivationPaymentNotice is the "pembayaran diterima" message for the buyer.
// It carries no internal ids and never the preview token.
type ActivationPaymentNotice struct {
	BuyerName   string
	BuyerEmail  string
	PageTitle   string
	PageURL     string // empty when the organization has no active host
	ActiveUntil time.Time
}

// ActivationPaidNotifier delivers the notice (implemented over
// internal/core/notification in internal/app).
type ActivationPaidNotifier interface {
	PaymentReceived(ctx context.Context, scope coretenant.Scope, notice ActivationPaymentNotice) error
}

// ActivationPublicLinks builds the page's public URL on the organization's
// active host; an empty string means no host is available.
type ActivationPublicLinks interface {
	PublicURL(ctx context.Context, scope coretenant.Scope, slug string, isHomepage bool) (string, error)
}

// ActivationSnapshotter stores a fresh published snapshot of the page's
// current content (publishService.SnapshotCurrent).
type ActivationSnapshotter interface {
	SnapshotCurrent(ctx context.Context, scope coretenant.Scope, pageID string, changeNote string) (domain.LandingPageVersion, error)
}

// ActivationPaidListener publishes an activated page once its invoice is paid.
// internal/app adapts it to the receivable listener registry, so landing does
// not import receivable.
type ActivationPaidListener struct {
	settler  repository.ActivationSettler
	notifier ActivationPaidNotifier
	links    ActivationPublicLinks
	snaps    ActivationSnapshotter
	now      func() time.Time
}

// WithSnapshotter makes every (re)publish by a payment take a fresh snapshot,
// so the live page is what the buyer previewed and not an older version.
func (l *ActivationPaidListener) WithSnapshotter(s ActivationSnapshotter) *ActivationPaidListener {
	l.snaps = s
	return l
}

func NewActivationPaidListener(settler repository.ActivationSettler, notifier ActivationPaidNotifier, links ActivationPublicLinks, now func() time.Time) *ActivationPaidListener {
	if now == nil {
		now = time.Now
	}
	return &ActivationPaidListener{settler: settler, notifier: notifier, links: links, now: now}
}

// InvoicePaid never returns an error: the receivable module only logs listener
// failures and the payment is already committed. An unknown invoice, another
// organization's invoice or an already settled activation changes nothing.
func (l *ActivationPaidListener) InvoicePaid(ctx context.Context, scope coretenant.Scope, invoiceID string) {
	if l == nil || l.settler == nil || !scope.IsValid() || strings.TrimSpace(invoiceID) == "" {
		return
	}
	paidAt := l.now().UTC()
	res, err := l.settler.SettleByInvoice(ctx, scope, invoiceID, paidAt)
	if err != nil {
		slog.ErrorContext(ctx, "activation payment: settle failed", "invoice_id", invoiceID, "error", err)
		return
	}
	if !res.Found || !res.Applied {
		return
	}
	if !res.Published {
		// Money was received but there is nothing to publish; never un-archive.
		slog.WarnContext(ctx, "activation payment received but page was not published",
			"activation_id", res.Activation.ID, "page_id", res.Activation.PageID, "page_found", res.PageFound)
		return
	}
	l.snapshot(ctx, scope, res)
	l.notify(ctx, scope, res)
}

// snapshot is best effort: the payment and publish are already committed.
func (l *ActivationPaidListener) snapshot(ctx context.Context, scope coretenant.Scope, res repository.ActivationSettlement) {
	if l.snaps == nil {
		return
	}
	if _, err := l.snaps.SnapshotCurrent(ctx, scope, res.Activation.PageID, "Terbit otomatis setelah pembayaran"); err != nil {
		slog.WarnContext(ctx, "activation payment: snapshot failed, page may serve an older version",
			"activation_id", res.Activation.ID, "page_id", res.Activation.PageID, "error", err)
	}
}

func (l *ActivationPaidListener) notify(ctx context.Context, scope coretenant.Scope, res repository.ActivationSettlement) {
	act := res.Activation
	if l.notifier == nil || act.BuyerEmail == nil || strings.TrimSpace(*act.BuyerEmail) == "" {
		// buyer_contact is not used: no WhatsApp path is wired for external buyers.
		slog.InfoContext(ctx, "activation payment: no buyer email, notification skipped", "activation_id", act.ID)
		return
	}
	url := ""
	if l.links != nil {
		var err error
		if url, err = l.links.PublicURL(ctx, scope, res.PageSlug, res.IsHomepage); err != nil {
			slog.WarnContext(ctx, "activation payment: public url unavailable", "activation_id", act.ID, "error", err)
			url = ""
		}
	}
	if url == "" {
		slog.WarnContext(ctx, "activation payment: no active host, notice sent without link", "activation_id", act.ID)
	}
	notice := ActivationPaymentNotice{
		BuyerName: act.BuyerName, BuyerEmail: strings.TrimSpace(*act.BuyerEmail),
		PageTitle: res.PageTitle, PageURL: url,
	}
	if res.ActiveUntil != nil {
		notice.ActiveUntil = *res.ActiveUntil
	}
	if err := l.notifier.PaymentReceived(ctx, scope, notice); err != nil {
		slog.WarnContext(ctx, "activation payment: notification failed, page stays published", "activation_id", act.ID, "error", err)
	}
}
