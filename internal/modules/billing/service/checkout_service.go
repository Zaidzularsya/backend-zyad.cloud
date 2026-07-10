package service

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	billing "zyad.cloud/internal/modules/billing"
	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/modules/billing/repository"
	"zyad.cloud/internal/platform/doku"
)

const checkoutPaymentDueDateMinutes = 60

// SetDokuCheckout wires the DOKU checkout client, the frontend base URL used
// to build post-payment callback redirects, and the public webhook URL sent
// as additional_info.override_notification_url on every payment.
func (s *PaymentService) SetDokuCheckout(client doku.Client, frontendURL string, notificationURL string) {
	s.dokuClient = client
	s.frontendURL = strings.TrimRight(strings.TrimSpace(frontendURL), "/")
	s.dokuNotificationURL = strings.TrimSpace(notificationURL)
}

// CreateCheckout creates (or reuses) a DOKU hosted checkout session for an
// open invoice belonging to the organization. The invoice UUID doubles as
// DOKU's order.invoice_number so notifications can be mapped back without a
// separate provider-reference lookup.
func (s *PaymentService) CreateCheckout(
	ctx context.Context,
	organizationID string,
	invoiceID string,
) (dto.CheckoutResponse, error) {
	if s.dokuClient == nil {
		return dto.CheckoutResponse{}, coreerrors.New(
			"PAYMENT_GATEWAY_NOT_CONFIGURED",
			"payment gateway is not configured",
			http.StatusServiceUnavailable,
		)
	}
	invoice, err := s.invoiceStore.FindByID(ctx, strings.TrimSpace(organizationID), strings.TrimSpace(invoiceID))
	if err != nil {
		return dto.CheckoutResponse{}, mapInvoiceError(err)
	}
	if invoice.IsPaid() || invoice.Status == model.InvoiceStatusPaid {
		return dto.CheckoutResponse{}, billing.PaymentAlreadyProcessedError()
	}
	if invoice.Status != model.InvoiceStatusOpen && invoice.Status != model.InvoiceStatusDraft {
		return dto.CheckoutResponse{}, validationError("invoice is not payable in its current status")
	}

	if response, found, err := s.findReusableCheckout(ctx, invoice); err != nil {
		return dto.CheckoutResponse{}, err
	} else if found {
		return response, nil
	}

	amount, err := wholeCurrencyAmount(invoice.TotalAmount)
	if err != nil {
		return dto.CheckoutResponse{}, validationError("invoice total amount is invalid for checkout")
	}
	if amount <= 0 {
		return dto.CheckoutResponse{}, validationError("invoice total amount must be positive for checkout")
	}

	payment, err := s.dokuClient.CreatePayment(ctx, doku.CreatePaymentRequest{
		InvoiceNumber:         invoice.ID,
		Amount:                amount,
		Currency:              currencyOrDefault(invoice.Currency, "IDR"),
		PaymentDueDateMinutes: checkoutPaymentDueDateMinutes,
		CallbackURL:           s.checkoutCallbackURL(invoice.ID),
		NotificationURL:       s.dokuNotificationURL,
	})
	if err != nil {
		return dto.CheckoutResponse{}, coreerrors.Wrap(
			"PAYMENT_CHECKOUT_FAILED",
			"failed to create payment checkout session",
			http.StatusBadGateway,
			err,
		)
	}

	providerReference := strings.TrimSpace(payment.TokenID)
	if providerReference == "" {
		providerReference = strings.TrimSpace(payment.SessionID)
	}
	expiredDate := payment.ExpiredDate.UTC().Format(time.RFC3339)
	if _, err := s.store.Create(ctx, repository.CreatePaymentParams{
		InvoiceID:         invoice.ID,
		OrganizationID:    invoice.OrganizationID,
		Provider:          model.PaymentProviderDoku,
		ProviderReference: providerReference,
		Status:            model.PaymentStatusPending,
		Amount:            invoice.TotalAmount,
		Currency:          currencyOrDefault(invoice.Currency, "IDR"),
		RawPayload: map[string]any{
			"payment_url":    payment.PaymentURL,
			"expired_date":   expiredDate,
			"invoice_number": invoice.ID,
			"token_id":       payment.TokenID,
			"session_id":     payment.SessionID,
		},
	}); err != nil {
		return dto.CheckoutResponse{}, err
	}

	return dto.CheckoutResponse{
		PaymentURL: payment.PaymentURL,
		Provider:   string(model.PaymentProviderDoku),
		ExpiresAt:  &expiredDate,
	}, nil
}

// findReusableCheckout returns an existing pending DOKU checkout URL for the
// invoice when its payment page has not expired yet, so repeated clicks do
// not create duplicate DOKU sessions.
func (s *PaymentService) findReusableCheckout(
	ctx context.Context,
	invoice model.Invoice,
) (dto.CheckoutResponse, bool, error) {
	payments, _, err := s.store.List(ctx, repository.PaymentListFilter{
		OrganizationID: invoice.OrganizationID,
		InvoiceID:      invoice.ID,
		Provider:       model.PaymentProviderDoku,
		Status:         model.PaymentStatusPending,
	})
	if err != nil {
		return dto.CheckoutResponse{}, false, err
	}
	now := s.now().UTC()
	for _, payment := range payments {
		// Simulated (NoopClient) sessions must never be reused: once real
		// credentials are configured, a lingering pending noop payment would
		// keep short-circuiting checkout back to the fake success URL.
		if sessionID, _ := metadataStringValue(payment.RawPayload, "session_id"); sessionID == "noop" {
			continue
		}
		paymentURL, _ := metadataStringValue(payment.RawPayload, "payment_url")
		expiredDateRaw, _ := metadataStringValue(payment.RawPayload, "expired_date")
		if paymentURL == "" || expiredDateRaw == "" {
			continue
		}
		expiredDate, parseErr := time.Parse(time.RFC3339, expiredDateRaw)
		if parseErr != nil || !expiredDate.After(now) {
			continue
		}
		expiresAt := expiredDate.UTC().Format(time.RFC3339)
		return dto.CheckoutResponse{
			PaymentURL: paymentURL,
			Provider:   string(model.PaymentProviderDoku),
			ExpiresAt:  &expiresAt,
		}, true, nil
	}
	return dto.CheckoutResponse{}, false, nil
}

// SyncCheckoutStatus actively reconciles an open invoice against DOKU's
// Check Status API. It is the fallback for environments where DOKU
// notifications never arrive: when DOKU reports SUCCESS the invoice settles
// through the same idempotent MarkInvoicePaidByID path the webhook uses
// (including subscription upgrade activation).
func (s *PaymentService) SyncCheckoutStatus(
	ctx context.Context,
	organizationID string,
	invoiceID string,
) (dto.CheckoutStatusResponse, error) {
	invoice, err := s.invoiceStore.FindByID(ctx, strings.TrimSpace(organizationID), strings.TrimSpace(invoiceID))
	if err != nil {
		return dto.CheckoutStatusResponse{}, mapInvoiceError(err)
	}
	response := dto.CheckoutStatusResponse{
		InvoiceID:     invoice.ID,
		InvoiceStatus: string(invoice.Status),
		Paid:          invoice.IsPaid() || invoice.Status == model.InvoiceStatusPaid,
	}
	if response.Paid {
		return response, nil
	}
	if s.dokuClient == nil {
		return response, nil
	}

	// Only query DOKU when a checkout session was actually created.
	payments, _, err := s.store.List(ctx, repository.PaymentListFilter{
		OrganizationID: invoice.OrganizationID,
		InvoiceID:      invoice.ID,
		Provider:       model.PaymentProviderDoku,
	})
	if err != nil {
		return dto.CheckoutStatusResponse{}, err
	}
	if len(payments) == 0 {
		return response, nil
	}

	status, err := s.dokuClient.CheckStatus(ctx, invoice.ID)
	if err != nil {
		return dto.CheckoutStatusResponse{}, coreerrors.Wrap(
			"PAYMENT_STATUS_CHECK_FAILED",
			"failed to check payment status with provider",
			http.StatusBadGateway,
			err,
		)
	}
	response.TransactionStatus = strings.ToUpper(strings.TrimSpace(status.Status))
	if !status.IsFinalSuccess() {
		return response, nil
	}

	var paidAt *string
	if parsed, parseErr := time.Parse(time.RFC3339, status.Date); parseErr == nil {
		formatted := parsed.UTC().Format(time.RFC3339)
		paidAt = &formatted
	}
	if _, err := s.MarkInvoicePaidByID(ctx, invoice.ID, dto.MarkInvoicePaidRequest{
		Provider:          string(model.PaymentProviderDoku),
		ProviderReference: firstNonEmpty(status.OriginalRequestID, "checkstatus-"+invoice.ID),
		PaymentMethod:     status.Channel,
		Amount:            status.Amount,
		Currency:          currencyOrDefault(invoice.Currency, "IDR"),
		PaidAt:            paidAt,
		RawPayload:        status.Raw,
	}); err != nil {
		var appErr *coreerrors.AppError
		if !errors.As(err, &appErr) || appErr.Code != billing.ErrCodePaymentAlreadyProcessed {
			return dto.CheckoutStatusResponse{}, err
		}
	}
	response.Paid = true
	response.InvoiceStatus = string(model.InvoiceStatusPaid)
	return response, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *PaymentService) checkoutCallbackURL(invoiceID string) string {
	if s.frontendURL == "" {
		return ""
	}
	return s.frontendURL + "/app/checkout/success?invoice=" + invoiceID
}

// wholeCurrencyAmount converts a decimal money string into a whole-unit
// amount (IDR has no minor units on DOKU), rounding half up.
func wholeCurrencyAmount(value string) (int64, error) {
	rat, err := moneyRat(value)
	if err != nil {
		return 0, err
	}
	if rat.Sign() < 0 {
		return 0, fmt.Errorf("money value must not be negative")
	}
	rounded := new(big.Rat).Add(rat, big.NewRat(1, 2))
	quotient := new(big.Int).Quo(rounded.Num(), rounded.Denom())
	if !quotient.IsInt64() {
		return 0, fmt.Errorf("money value is out of range")
	}
	return quotient.Int64(), nil
}
