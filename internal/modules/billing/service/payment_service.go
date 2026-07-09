package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	billing "zyad.cloud/internal/modules/billing"
	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/modules/billing/repository"
)

type PaymentStore interface {
	Create(ctx context.Context, params repository.CreatePaymentParams) (model.Payment, error)
	FindByID(ctx context.Context, organizationID string, id string) (model.Payment, error)
	FindEventByProviderEventID(
		ctx context.Context,
		provider model.PaymentProvider,
		providerEventID string,
	) (model.PaymentEvent, error)
	List(ctx context.Context, filter repository.PaymentListFilter) ([]model.Payment, int64, error)
	CreateEvent(ctx context.Context, params repository.PaymentEventParams) (model.PaymentEvent, error)
}

type PaymentInvoiceStore interface {
	FindByID(ctx context.Context, organizationID string, id string) (model.Invoice, error)
	FindByIDUnscoped(ctx context.Context, id string) (model.Invoice, error)
	UpdateStatus(ctx context.Context, params repository.UpdateInvoiceStatusParams) (model.Invoice, error)
}

// SubscriptionUpgradeInvoice is the minimal invoice shape the subscription
// domain needs to activate a plan upgrade after payment. It intentionally
// mirrors subscriptionservice.SubscriptionInvoice's fields so billing's own
// model.Invoice can be adapted to it without billing importing subscription's
// model package (and vice versa, avoiding an import cycle).
type SubscriptionUpgradeInvoice struct {
	OrganizationID string
	SubscriptionID *string
	Metadata       map[string]any
}

type PaymentSubscriptionUpgradeActivator interface {
	ActivateUpgradeByInvoice(ctx context.Context, invoice SubscriptionUpgradeInvoice, paidAt time.Time) error
}

type PaymentService struct {
	store                PaymentStore
	invoiceStore         PaymentInvoiceStore
	subscriptionUpgrades PaymentSubscriptionUpgradeActivator
	now                  func() time.Time
}

func NewPaymentService(
	store PaymentStore,
	invoiceStore PaymentInvoiceStore,
	subscriptionUpgrades PaymentSubscriptionUpgradeActivator,
) *PaymentService {
	return &PaymentService{
		store:                store,
		invoiceStore:         invoiceStore,
		subscriptionUpgrades: subscriptionUpgrades,
		now:                  time.Now,
	}
}

func (s *PaymentService) FindByID(ctx context.Context, organizationID string, id string) (dto.PaymentResponse, error) {
	payment, err := s.store.FindByID(ctx, strings.TrimSpace(organizationID), strings.TrimSpace(id))
	if err != nil {
		return dto.PaymentResponse{}, err
	}
	return paymentResponse(payment), nil
}

func (s *PaymentService) RecordProviderEvent(
	ctx context.Context,
	request dto.RecordPaymentProviderEventRequest,
) (dto.PaymentEventResponse, error) {
	provider := model.PaymentProvider(strings.TrimSpace(request.Provider))
	if !provider.IsValid() || provider == model.PaymentProviderManual {
		return dto.PaymentEventResponse{}, validationError("payment provider is invalid")
	}
	providerEventID := strings.TrimSpace(request.ProviderEventID)
	if providerEventID == "" {
		return dto.PaymentEventResponse{}, validationError("provider event id is required")
	}
	eventType := strings.TrimSpace(strings.ToLower(request.EventType))
	if eventType == "" {
		return dto.PaymentEventResponse{}, validationError("payment event type is required")
	}
	processedAt, err := parseOptionalTime(request.ProcessedAt)
	if err != nil {
		return dto.PaymentEventResponse{}, err
	}

	event, err := s.store.CreateEvent(ctx, repository.PaymentEventParams{
		PaymentID:       stringPointer(request.PaymentID),
		InvoiceID:       stringPointer(request.InvoiceID),
		Provider:        provider,
		Type:            eventType,
		ProviderEventID: providerEventID,
		Payload:         request.Payload,
		ProcessedAt:     processedAt,
	})
	if err != nil {
		if isDuplicatePaymentEventError(err) {
			existing, findErr := s.store.FindEventByProviderEventID(ctx, provider, providerEventID)
			if findErr == nil {
				return paymentEventResponse(existing), nil
			}
			return dto.PaymentEventResponse{}, billing.PaymentEventAlreadyProcessedError()
		}
		return dto.PaymentEventResponse{}, err
	}
	return paymentEventResponse(event), nil
}

func (s *PaymentService) MarkInvoicePaid(
	ctx context.Context,
	organizationID string,
	invoiceID string,
	request dto.MarkInvoicePaidRequest,
) (dto.PaymentResponse, error) {
	invoice, err := s.invoiceStore.FindByID(ctx, strings.TrimSpace(organizationID), strings.TrimSpace(invoiceID))
	if err != nil {
		return dto.PaymentResponse{}, mapInvoiceError(err)
	}
	existing, found, err := s.findPaidPayment(ctx, invoice.OrganizationID, invoice.ID)
	if err != nil {
		return dto.PaymentResponse{}, err
	}
	if found {
		if !invoice.IsPaid() {
			paidAt := existing.PaidAt
			if paidAt == nil {
				now := s.now().UTC()
				paidAt = &now
			}
			if _, err := s.invoiceStore.UpdateStatus(ctx, repository.UpdateInvoiceStatusParams{
				ID:             invoice.ID,
				OrganizationID: invoice.OrganizationID,
				Status:         model.InvoiceStatusPaid,
				PaidAt:         paidAt,
			}); err != nil {
				return dto.PaymentResponse{}, mapInvoiceError(err)
			}
			if err := s.activateInvoiceUpgrade(ctx, invoice, *paidAt); err != nil {
				return dto.PaymentResponse{}, err
			}
		}
		return paymentResponse(existing), nil
	}
	if invoice.IsPaid() {
		return dto.PaymentResponse{}, billing.PaymentAlreadyProcessedError()
	}

	params, paidAt, err := s.paymentParams(invoice, request)
	if err != nil {
		return dto.PaymentResponse{}, err
	}
	payment, err := s.store.Create(ctx, params)
	if err != nil {
		return dto.PaymentResponse{}, err
	}
	if _, err := s.store.CreateEvent(ctx, repository.PaymentEventParams{
		PaymentID:       &payment.ID,
		InvoiceID:       &invoice.ID,
		Provider:        payment.Provider,
		Type:            "manual_payment_recorded",
		ProviderEventID: paymentEventID(payment),
		Payload:         request.RawPayload,
		ProcessedAt:     paidAt,
	}); err != nil {
		return dto.PaymentResponse{}, err
	}
	if _, err := s.invoiceStore.UpdateStatus(ctx, repository.UpdateInvoiceStatusParams{
		ID:             invoice.ID,
		OrganizationID: invoice.OrganizationID,
		Status:         model.InvoiceStatusPaid,
		PaidAt:         paidAt,
	}); err != nil {
		return dto.PaymentResponse{}, mapInvoiceError(err)
	}
	if err := s.activateInvoiceUpgrade(ctx, invoice, *paidAt); err != nil {
		return dto.PaymentResponse{}, err
	}
	return paymentResponse(payment), nil
}

func (s *PaymentService) MarkInvoicePaidByID(
	ctx context.Context,
	invoiceID string,
	request dto.MarkInvoicePaidRequest,
) (dto.PaymentResponse, error) {
	invoice, err := s.invoiceStore.FindByIDUnscoped(ctx, strings.TrimSpace(invoiceID))
	if err != nil {
		return dto.PaymentResponse{}, mapInvoiceError(err)
	}
	return s.MarkInvoicePaid(ctx, invoice.OrganizationID, invoice.ID, request)
}

func (s *PaymentService) findPaidPayment(
	ctx context.Context,
	organizationID string,
	invoiceID string,
) (model.Payment, bool, error) {
	payments, _, err := s.store.List(ctx, repository.PaymentListFilter{
		OrganizationID: organizationID,
		InvoiceID:      invoiceID,
		Status:         model.PaymentStatusPaid,
		Limit:          1,
	})
	if err != nil {
		return model.Payment{}, false, err
	}
	if len(payments) == 0 {
		return model.Payment{}, false, nil
	}
	return payments[0], true, nil
}

func (s *PaymentService) paymentParams(
	invoice model.Invoice,
	request dto.MarkInvoicePaidRequest,
) (repository.CreatePaymentParams, *time.Time, error) {
	provider := model.PaymentProvider(strings.TrimSpace(request.Provider))
	if provider == "" {
		provider = model.PaymentProviderManual
	}
	if !provider.IsValid() {
		return repository.CreatePaymentParams{}, nil, validationError("payment provider is invalid")
	}
	paidAt, err := parseOptionalTime(request.PaidAt)
	if err != nil {
		return repository.CreatePaymentParams{}, nil, err
	}
	if paidAt == nil {
		now := s.now().UTC()
		paidAt = &now
	}
	amount := strings.TrimSpace(request.Amount)
	if amount == "" {
		amount = invoice.TotalAmount
	}
	if _, err := moneyRat(amount); err != nil {
		return repository.CreatePaymentParams{}, nil, validationError("payment amount is invalid")
	}
	currency := currencyOrDefault(request.Currency, invoice.Currency)
	providerReference := strings.TrimSpace(request.ProviderReference)
	if providerReference == "" && provider == model.PaymentProviderManual {
		providerReference = "manual-" + invoice.ID
	}
	return repository.CreatePaymentParams{
		InvoiceID:         invoice.ID,
		OrganizationID:    invoice.OrganizationID,
		Provider:          provider,
		ProviderReference: providerReference,
		PaymentMethod:     strings.TrimSpace(request.PaymentMethod),
		Status:            model.PaymentStatusPaid,
		Amount:            amount,
		Currency:          currency,
		PaidAt:            paidAt,
		RawPayload:        request.RawPayload,
	}, paidAt, nil
}

func paymentEventID(payment model.Payment) string {
	if strings.TrimSpace(payment.ProviderReference) == "" {
		return ""
	}
	return payment.ProviderReference + ":paid"
}

func (s *PaymentService) activateInvoiceUpgrade(
	ctx context.Context,
	invoice model.Invoice,
	paidAt time.Time,
) error {
	if s.subscriptionUpgrades == nil {
		return nil
	}
	if !isUpgradeRequestMetadata(invoice.Metadata) {
		return nil
	}
	return s.subscriptionUpgrades.ActivateUpgradeByInvoice(ctx, SubscriptionUpgradeInvoice{
		OrganizationID: invoice.OrganizationID,
		SubscriptionID: invoice.SubscriptionID,
		Metadata:       invoice.Metadata,
	}, paidAt)
}

func isUpgradeRequestMetadata(metadata map[string]any) bool {
	value, ok := metadataStringValue(metadata, "billing_action")
	return ok && value == "upgrade_request"
}

func metadataStringValue(metadata map[string]any, key string) (string, bool) {
	if metadata == nil {
		return "", false
	}
	value, ok := metadata[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	return text, true
}

func isDuplicatePaymentEventError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
