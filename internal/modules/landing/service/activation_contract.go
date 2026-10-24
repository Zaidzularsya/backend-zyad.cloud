package service

import (
	"context"
	"time"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

// ActivationPreviewURLTemplate is the public preview path; {token} is the raw
// token, which is only returned once when the activation is created.
const ActivationPreviewURLTemplate = "/api/v1/public/landing/preview/{token}"

// ActivationReader exposes page activation state to the admin page API.
type ActivationReader interface {
	// OpenActivationsByPages returns the open activation of each page, keyed by page ID.
	OpenActivationsByPages(ctx context.Context, scope tenant.Scope, pageIDs []string) (map[string]domain.LandingPageActivation, error)
}

// Feature keys required to sell a page through an activation invoice.
const (
	FeaturePageActivation = "landing.page_activation"
)

// ActivationInput is the reseller's "Create invoice" form. Price is decimal
// text (never float). At most one of InvoiceDueHours/InvoiceDueDays is set;
// neither means one hour.
type ActivationInput struct {
	BuyerName, BuyerContact, BuyerEmail string
	Price                               string
	DurationValue                       int
	DurationUnit                        domain.ActivationDurationUnit
	InvoiceDueHours, InvoiceDueDays     *int
}

// ActivationResult carries the raw-token preview URL, returned only here.
type ActivationResult struct {
	Activation domain.LandingPageActivation
	PreviewURL string
	PaymentURL string
}

type ActivationService interface {
	CreateInvoice(ctx context.Context, scope tenant.Scope, pageID string, in ActivationInput, userID string) (ActivationResult, error)
}

// ActivationFeatureGate checks organization entitlements; any error denies.
type ActivationFeatureGate interface {
	RequireFeature(ctx context.Context, organizationID, featureKey string) error
}

// ActivationInvoiceRequest is what the billing port needs to bill the buyer.
// It never contains the raw preview token.
type ActivationInvoiceRequest struct {
	ActivationID                        string // also the invoice idempotency key
	BuyerName, BuyerContact, BuyerEmail string
	Description                         string
	Price                               string
	DueAt                               time.Time
}

type ActivationInvoice struct {
	InvoiceID  string
	PaymentURL string
}

// ActivationBilling issues the activation invoice in the reseller's own
// organization (implemented over the receivable module in internal/app, so
// landing does not import receivable).
type ActivationBilling interface {
	// IssueInvoice may return a non-empty InvoiceID together with an error
	// when the invoice exists but a later step failed; the caller voids it.
	IssueInvoice(ctx context.Context, scope tenant.Scope, req ActivationInvoiceRequest, actorUserID string) (ActivationInvoice, error)
	VoidInvoice(ctx context.Context, scope tenant.Scope, invoiceID, reason, actorUserID string) error
}

// ActivationLinks builds the absolute public preview URL on the reseller's
// tenant host. Nil falls back to the relative ActivationPreviewURLTemplate.
type ActivationLinks interface {
	PreviewURL(ctx context.Context, scope tenant.Scope, rawToken string) (string, error)
}

type ActivationServiceDeps struct {
	Activations repository.ActivationRepository
	Pages       repository.PageRepository
	Features    ActivationFeatureGate
	Billing     ActivationBilling
	Links       ActivationLinks
	Now         func() time.Time
}
