package app

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"zyad.cloud/internal/core/businesstime"
	coreerrors "zyad.cloud/internal/core/errors"
	notificationdomain "zyad.cloud/internal/core/notification/domain"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	coretenant "zyad.cloud/internal/core/tenant"
	landingrepo "zyad.cloud/internal/modules/landing/repository"
	landingservice "zyad.cloud/internal/modules/landing/service"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/pricing"
)

// activationInvoiceChecker answers the receivable online-payment gate: does
// this invoice of the scope's organization belong to a landing activation
// (any status, so a late payment on a void activation stays payable).
type activationInvoiceChecker struct {
	activations landingrepo.ActivationRepository
}

func (c activationInvoiceChecker) IsActivationInvoice(ctx context.Context, scope coretenant.Scope, invoiceID string) (bool, error) {
	if c.activations == nil {
		return false, errors.New("activation repository is not configured")
	}
	return c.activations.ExistsForInvoice(ctx, scope, invoiceID)
}

var _ receivableservice.ActivationInvoiceChecker = activationInvoiceChecker{}

// receivableActivationBilling issues landing activation invoices through the
// receivable module, always in the reseller's own organization (scope).
type receivableActivationBilling struct {
	accounts receivableservice.AccountService
	invoices receivableservice.InvoiceService
}

func (b receivableActivationBilling) IssueInvoice(ctx context.Context, scope coretenant.Scope, req landingservice.ActivationInvoiceRequest, actorUserID string) (landingservice.ActivationInvoice, error) {
	// Plain account (no CRM source): receivable accounts only know crm_contact sources.
	account, err := b.accounts.Create(ctx, scope, receivableservice.AccountInput{
		Name: req.BuyerName, Email: req.BuyerEmail, Phone: req.BuyerContact,
	}, actorUserID)
	if err != nil {
		return landingservice.ActivationInvoice{}, activationBillingError(err)
	}
	due := req.DueAt
	inv, err := b.invoices.CreateAndIssue(ctx, scope, receivableservice.InvoiceInput{
		AccountID:  account.ID,
		SourceType: receivabledomain.SourceManual,
		// Source + key = activation id: CreateAndIssue is idempotent per activation.
		SourceID: req.ActivationID, IdempotencyKey: req.ActivationID,
		DueDate: &due,
		// Channels empty: the organization's default channels apply.
		Lines: []receivableservice.LineInput{{LineInput: pricing.LineInput{
			Description: req.Description, Quantity: "1", UnitPrice: req.Price,
		}}},
	}, actorUserID)
	if err != nil {
		return landingservice.ActivationInvoice{}, activationBillingError(err)
	}
	url, _, err := b.invoices.Link(ctx, scope, inv.ID, actorUserID)
	if err != nil {
		return landingservice.ActivationInvoice{InvoiceID: inv.ID}, activationBillingError(err)
	}
	return landingservice.ActivationInvoice{InvoiceID: inv.ID, PaymentURL: url}, nil
}

func (b receivableActivationBilling) VoidInvoice(ctx context.Context, scope coretenant.Scope, invoiceID, reason, actorUserID string) error {
	_, err := b.invoices.Void(ctx, scope, invoiceID, reason, actorUserID)
	return err
}

// activationBillingError keeps API errors (quota, ...) and maps receivable
// validation errors to 422; anything else is an opaque failure.
func activationBillingError(err error) error {
	var appErr *coreerrors.AppError
	if errors.As(err, &appErr) {
		return err
	}
	if errors.Is(err, receivableservice.ErrInvalidAccount) || errors.Is(err, receivableservice.ErrInvalidInvoice) {
		return coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity)
	}
	return coreerrors.Wrap("ACTIVATION_INVOICE_FAILED", "activation invoice could not be created", http.StatusBadGateway, err)
}

var _ landingservice.ActivationBilling = receivableActivationBilling{}

// activeDomainLister is the subset of the organization domain repository used
// to find the reseller's public host.
type activeDomainLister interface {
	ListByOrganization(ctx context.Context, organizationID string, filter organizationrepo.DomainListFilter) ([]organizationmodel.OrganizationDomain, int64, error)
}

// activationPreviewLinks builds the preview URL on the organization's active
// public host (primary first), because the public preview route resolves the
// tenant from the request host. Without an active host it falls back to the
// relative path.
type activationPreviewLinks struct {
	domains activeDomainLister
}

func (l activationPreviewLinks) PreviewURL(ctx context.Context, scope coretenant.Scope, rawToken string) (string, error) {
	path := strings.Replace(landingservice.ActivationPreviewURLTemplate, "{token}", rawToken, 1)
	domains, _, err := l.domains.ListByOrganization(ctx, scope.OrganizationID(), organizationrepo.DomainListFilter{
		Status: organizationmodel.DomainStatusActive, Limit: 20,
	})
	if err != nil {
		return "", err
	}
	for _, d := range domains { // ordered is_primary DESC, created_at ASC
		if host := strings.TrimSpace(d.CanonicalHost); host != "" && d.OrganizationID == scope.OrganizationID() {
			return "https://" + host + path, nil
		}
	}
	return path, nil
}

var _ landingservice.ActivationLinks = activationPreviewLinks{}

// activationPublicLinks builds a page's public URL on the organization's
// active host (primary first); empty when the organization has none.
type activationPublicLinks struct {
	domains activeDomainLister
}

func (l activationPublicLinks) PublicURL(ctx context.Context, scope coretenant.Scope, slug string, isHomepage bool) (string, error) {
	domains, _, err := l.domains.ListByOrganization(ctx, scope.OrganizationID(), organizationrepo.DomainListFilter{
		Status: organizationmodel.DomainStatusActive, Limit: 20,
	})
	if err != nil {
		return "", err
	}
	for _, d := range domains {
		if host := strings.TrimSpace(d.CanonicalHost); host != "" && d.OrganizationID == scope.OrganizationID() {
			if isHomepage || slug == "" {
				return "https://" + host + "/", nil
			}
			return "https://" + host + "/" + slug, nil
		}
	}
	return "", nil
}

var _ landingservice.ActivationPublicLinks = activationPublicLinks{}

// EventLandingActivationPaid is the notification event for the buyer.
const EventLandingActivationPaid = "landing.activation_paid"

type activationNoticePublisher interface {
	Publish(ctx context.Context, event notificationpublisher.Event) (notificationdomain.OutboxEvent, error)
}

// activationPaidNotifier enqueues the buyer email through the notification outbox.
type activationPaidNotifier struct {
	publisher activationNoticePublisher
	appName   string
	locale    string
}

func (n activationPaidNotifier) PaymentReceived(ctx context.Context, scope coretenant.Scope, in landingservice.ActivationPaymentNotice) error {
	line := ""
	if in.PageURL != "" {
		line = "Halaman Anda dapat dibuka di " + in.PageURL + "."
	}
	_, err := n.publisher.Publish(ctx, notificationpublisher.Event{
		Type: EventLandingActivationPaid, OrganizationID: scope.OrganizationID(),
		Recipient: notificationdomain.NotificationRecipient{Type: "external", Name: in.BuyerName, Email: in.BuyerEmail},
		Payload: map[string]any{
			"app_name": n.appName, "buyer_name": in.BuyerName, "page_title": in.PageTitle,
			"active_until": businesstime.DayOf(in.ActiveUntil).Format("02-01-2006"), "page_link_line": line,
		},
		Locale: n.locale,
	})
	return err
}

var _ landingservice.ActivationPaidNotifier = activationPaidNotifier{}

// landingActivationReceivableListener adapts the landing listener to the
// receivable registry; contract events are of no interest to landing.
type landingActivationReceivableListener struct {
	paid *landingservice.ActivationPaidListener
}

func (l landingActivationReceivableListener) InvoicePaid(ctx context.Context, scope coretenant.Scope, inv receivableservice.InvoiceRef) {
	l.paid.InvoicePaid(ctx, scope, inv.ID)
}

// InvoicePaidReplay opts landing into resent provider notifications: settle is
// idempotent (a paid activation is a no-op with no second notice), so a settle
// that failed after the payment committed is retried by resending the notification.
func (l landingActivationReceivableListener) InvoicePaidReplay(ctx context.Context, scope coretenant.Scope, inv receivableservice.InvoiceRef) {
	l.paid.InvoicePaid(ctx, scope, inv.ID)
}
func (landingActivationReceivableListener) ContractCreated(context.Context, coretenant.Scope, receivableservice.ContractRef) {
}
func (landingActivationReceivableListener) ContractEnded(context.Context, coretenant.Scope, receivableservice.ContractRef) {
}

var (
	_ receivableservice.Listener           = landingActivationReceivableListener{}
	_ receivableservice.ReplayableListener = landingActivationReceivableListener{}
)
