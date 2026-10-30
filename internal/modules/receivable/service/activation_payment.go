package service

import (
	"context"
	"log/slog"

	coretenant "zyad.cloud/internal/core/tenant"
)

// activationPaymentFeature lets a reseller's landing page activation invoices
// be paid online without receivable.online_payment (spec 2C decision 2).
const activationPaymentFeature = "landing.page_activation"

// ActivationInvoiceChecker reports whether an invoice of the scope's
// organization belongs to a landing page activation. It is implemented in
// internal/app so receivable does not import the landing module.
type ActivationInvoiceChecker interface {
	IsActivationInvoice(ctx context.Context, scope coretenant.Scope, invoiceID string) (bool, error)
}

// onlinePaymentAllowed is the single online-payment gate. The existing rule
// (receivable.online_payment) is checked first and unchanged; only when it
// fails may an activation invoice pass through landing.page_activation. Any
// error denies (fail closed).
func onlinePaymentAllowed(ctx context.Context, gate Entitlements, activations ActivationInvoiceChecker, scope coretenant.Scope, invoiceID string) bool {
	if gate == nil {
		return false
	}
	orgID := scope.OrganizationID()
	if gate.RequireFeature(ctx, orgID, onlinePaymentFeature) == nil {
		return true
	}
	if activations == nil {
		return false
	}
	isActivation, err := activations.IsActivationInvoice(ctx, scope, invoiceID)
	if err != nil {
		slog.WarnContext(ctx, "receivable: activation invoice check failed, online payment denied", "invoice_id", invoiceID, "error", err)
		return false
	}
	return isActivation && gate.RequireFeature(ctx, orgID, activationPaymentFeature) == nil
}

// WithActivationInvoices enables online payment of landing activation
// invoices for organizations that have landing.page_activation.
func (p *OnlinePayment) WithActivationInvoices(checker ActivationInvoiceChecker) *OnlinePayment {
	p.activations = checker
	return p
}

// WithActivationInvoices: see OnlinePayment.WithActivationInvoices (CanPay).
func (s *PublicInvoiceService) WithActivationInvoices(checker ActivationInvoiceChecker) *PublicInvoiceService {
	s.activations = checker
	return s
}
