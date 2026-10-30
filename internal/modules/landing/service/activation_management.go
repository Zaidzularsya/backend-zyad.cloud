package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

// Reseller-facing state of an activation invoice in the invoice list.
const (
	ActivationEntryAwaitingPayment = "awaiting_payment" // open, invoice not yet due
	ActivationEntryExpired         = "expired"          // open, invoice past its due time (preview closed)
	ActivationEntryPaid            = "paid"
	ActivationEntryVoid            = "void"
)

const (
	defaultActivationListPerPage = 20
	maxActivationListPerPage     = 100
	// renewing usually follows a customer who is slow to pay, so it defaults to a day, not an hour.
	defaultActivationRenewDue = 24 * time.Hour
)

var (
	errActivationNotFound = coreerrors.New("ACTIVATION_NOT_FOUND", "Invoice aktivasi tidak ditemukan.", http.StatusNotFound)
	errActivationNotOpen  = coreerrors.New("ACTIVATION_NOT_OPEN",
		"Invoice ini sudah dibayar, dibatalkan, atau belum siap sehingga link-nya tidak bisa diperbarui.", http.StatusConflict)
)

// ActivationPaymentLinks returns the public payment link of an activation invoice. The link is stored
// encrypted by the receivable module, so the same link can be shown again at any time.
type ActivationPaymentLinks interface {
	PaymentURL(ctx context.Context, scope tenant.Scope, invoiceID string) (string, error)
}

type ActivationListQuery struct {
	Status  string // "", "open", "paid" or "void"
	Page    int
	PerPage int
}

type ActivationListEntry struct {
	Activation      domain.LandingPageActivation
	PageTitle       string
	PageSlug        string
	PageStatus      domain.PageStatus
	PageActiveUntil *time.Time
	State           string
	PaymentURL      string // empty for void invoices or when the link could not be built
}

type ActivationList struct {
	Items   []ActivationListEntry
	Total   int64
	Page    int
	PerPage int
}

// ActivationRenewInput is how long the renewed preview window lasts: hours or days (default 24 hours).
type ActivationRenewInput struct {
	InvoiceDueHours, InvoiceDueDays *int
}

// ActivationRenewResult carries the NEW raw-token preview URL, returned only here.
type ActivationRenewResult struct {
	Activation domain.LandingPageActivation
	PreviewURL string
	PaymentURL string
}

// ActivationManager is the reseller's invoice dashboard: list the activation invoices and renew the
// preview link of one that is still open.
type ActivationManager interface {
	List(ctx context.Context, scope tenant.Scope, q ActivationListQuery) (ActivationList, error)
	Renew(ctx context.Context, scope tenant.Scope, id string, in ActivationRenewInput, userID string) (ActivationRenewResult, error)
}

type ActivationManagerDeps struct {
	Activations repository.ActivationRepository
	Features    ActivationFeatureGate
	Links       ActivationLinks
	Payments    ActivationPaymentLinks
	Now         func() time.Time
}

type activationManager struct {
	activations repository.ActivationRepository
	features    ActivationFeatureGate
	links       ActivationLinks
	payments    ActivationPaymentLinks
	now         func() time.Time
}

func NewActivationManager(d ActivationManagerDeps) ActivationManager {
	now := d.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &activationManager{activations: d.Activations, features: d.Features, links: d.Links, payments: d.Payments, now: now}
}

func activationEntryState(a domain.LandingPageActivation, now time.Time) string {
	switch a.Status {
	case domain.ActivationStatusPaid:
		return ActivationEntryPaid
	case domain.ActivationStatusVoid:
		return ActivationEntryVoid
	}
	if a.PreviewableAt(now) {
		return ActivationEntryAwaitingPayment
	}
	return ActivationEntryExpired
}

// List needs no selling entitlement: a reseller whose plan lapsed can still read the invoice history.
func (s *activationManager) List(ctx context.Context, scope tenant.Scope, q ActivationListQuery) (ActivationList, error) {
	if !scope.IsValid() {
		return ActivationList{}, tenant.ErrInvalidScope
	}
	status := domain.ActivationStatus(q.Status)
	if q.Status != "" && !status.IsValid() {
		return ActivationList{}, activationValidation("Status harus open, paid, atau void.")
	}
	perPage := q.PerPage
	if perPage < 1 {
		perPage = defaultActivationListPerPage
	}
	perPage = min(perPage, maxActivationListPerPage)
	page := max(q.Page, 1)

	items, total, err := s.activations.ListByOrganization(ctx, scope, repository.ActivationListFilter{
		Status: status, Limit: perPage, Offset: (page - 1) * perPage,
	})
	if err != nil {
		return ActivationList{}, err
	}
	now := s.now().UTC()
	out := ActivationList{Items: make([]ActivationListEntry, 0, len(items)), Total: total, Page: page, PerPage: perPage}
	for _, it := range items {
		entry := ActivationListEntry{
			Activation: it.Activation, PageTitle: it.PageTitle, PageSlug: it.PageSlug,
			PageStatus: it.PageStatus, PageActiveUntil: it.PageActiveUntil,
			State: activationEntryState(it.Activation, now),
		}
		if it.Activation.InvoiceID != nil && it.Activation.Status != domain.ActivationStatusVoid && s.payments != nil {
			url, err := s.payments.PaymentURL(ctx, scope, *it.Activation.InvoiceID)
			if err != nil {
				slog.WarnContext(ctx, "landing: activation payment link unavailable", "activation_id", it.Activation.ID, "error", err)
			} else {
				entry.PaymentURL = url
			}
		}
		out.Items = append(out.Items, entry)
	}
	return out, nil
}

func (s *activationManager) requireFeatures(ctx context.Context, scope tenant.Scope) error {
	if s.features == nil {
		return errActivationFeatureDisabled
	}
	for _, key := range []string{domain.FeatureLandingEnabled, FeaturePageActivation} {
		if err := s.features.RequireFeature(ctx, scope.OrganizationID(), key); err != nil {
			return err
		}
	}
	return nil
}

// Renew gives an open activation a NEW preview link and a fresh preview window. The raw token is never
// stored, so a link cannot be shown again; renewing is how the reseller gets one to resend. The old link
// stops working. The payment link does not change: it outlives the invoice due date and an overdue
// invoice stays payable. The link is built before the token is rotated, so a reseller without a public
// host keeps the old token.
func (s *activationManager) Renew(ctx context.Context, scope tenant.Scope, id string, in ActivationRenewInput, _ string) (ActivationRenewResult, error) {
	if !scope.IsValid() {
		return ActivationRenewResult{}, tenant.ErrInvalidScope
	}
	if err := s.requireFeatures(ctx, scope); err != nil {
		return ActivationRenewResult{}, err
	}
	dueAfter, err := activationDueAfter(in.InvoiceDueHours, in.InvoiceDueDays, defaultActivationRenewDue)
	if err != nil {
		return ActivationRenewResult{}, err
	}
	current, err := s.activations.FindByID(ctx, scope, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ActivationRenewResult{}, errActivationNotFound
	}
	if err != nil {
		return ActivationRenewResult{}, err
	}
	if current.Status != domain.ActivationStatusOpen || current.InvoiceID == nil {
		return ActivationRenewResult{}, errActivationNotOpen
	}

	rawToken, tokenHash, err := domain.NewActivationPreviewToken()
	if err != nil {
		return ActivationRenewResult{}, err
	}
	previewURL, err := s.previewURL(ctx, scope, rawToken)
	if err != nil {
		return ActivationRenewResult{}, err
	}
	rotated, err := s.activations.RotatePreview(ctx, scope, current.ID, tokenHash, s.now().UTC().Add(dueAfter))
	if errors.Is(err, pgx.ErrNoRows) {
		return ActivationRenewResult{}, errActivationNotOpen // paid or voided just before the update
	}
	if err != nil {
		return ActivationRenewResult{}, err
	}

	result := ActivationRenewResult{Activation: rotated, PreviewURL: previewURL}
	if s.payments != nil {
		url, err := s.payments.PaymentURL(ctx, scope, *current.InvoiceID)
		if err != nil {
			slog.WarnContext(ctx, "landing: activation payment link unavailable after renew", "activation_id", current.ID, "error", err)
		} else {
			result.PaymentURL = url
		}
	}
	return result, nil
}

func (s *activationManager) previewURL(ctx context.Context, scope tenant.Scope, token string) (string, error) {
	if s.links == nil {
		return strings.Replace(ActivationPreviewURLTemplate, "{token}", token, 1), nil
	}
	return s.links.PreviewURL(ctx, scope, token)
}
