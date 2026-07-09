package billing

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
)

const (
	ErrCodeInvoiceNotFound              = "INVOICE_NOT_FOUND"
	ErrCodePaymentAlreadyProcessed      = "PAYMENT_ALREADY_PROCESSED"
	ErrCodePaymentEventAlreadyProcessed = "PAYMENT_EVENT_ALREADY_PROCESSED"
	ErrCodeBillingEntitlementSyncFailed = "BILLING_ENTITLEMENT_SYNC_FAILED"
)

func InvoiceNotFoundError() error {
	return coreerrors.New(ErrCodeInvoiceNotFound, "billing invoice not found", http.StatusNotFound)
}

func PaymentAlreadyProcessedError() error {
	return coreerrors.New(ErrCodePaymentAlreadyProcessed, "billing payment is already processed", http.StatusConflict)
}

func PaymentEventAlreadyProcessedError() error {
	return coreerrors.New(ErrCodePaymentEventAlreadyProcessed, "billing payment event is already processed", http.StatusConflict)
}
