package billing

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
)

const (
	ErrCodeSubscriptionNotFound         = "SUBSCRIPTION_NOT_FOUND"
	ErrCodeSubscriptionInactive         = "SUBSCRIPTION_INACTIVE"
	ErrCodeSubscriptionSuspended        = "SUBSCRIPTION_SUSPENDED"
	ErrCodeFeatureNotEnabled            = "FEATURE_NOT_ENABLED"
	ErrCodeQuotaExceeded                = "QUOTA_EXCEEDED"
	ErrCodePlanNotFound                 = "PLAN_NOT_FOUND"
	ErrCodePlanPriceNotFound            = "PLAN_PRICE_NOT_FOUND"
	ErrCodeFeatureNotFound              = "FEATURE_NOT_FOUND"
	ErrCodeInvoiceNotFound              = "INVOICE_NOT_FOUND"
	ErrCodePaymentAlreadyProcessed      = "PAYMENT_ALREADY_PROCESSED"
	ErrCodeInvalidBillingStatus         = "INVALID_BILLING_STATUS"
	ErrCodeBillingEntitlementSyncFailed = "BILLING_ENTITLEMENT_SYNC_FAILED"
	ErrCodePaymentEventAlreadyProcessed = "PAYMENT_EVENT_ALREADY_PROCESSED"
)

func PlanNotFoundError() error {
	return coreerrors.New(ErrCodePlanNotFound, "billing plan not found", http.StatusNotFound)
}

func PlanPriceNotFoundError() error {
	return coreerrors.New(ErrCodePlanPriceNotFound, "billing plan price not found", http.StatusNotFound)
}

func FeatureNotFoundError() error {
	return coreerrors.New(ErrCodeFeatureNotFound, "billing feature not found", http.StatusNotFound)
}

func SubscriptionNotFoundError() error {
	return coreerrors.New(ErrCodeSubscriptionNotFound, "billing subscription not found", http.StatusNotFound)
}

func SubscriptionInactiveError() error {
	return coreerrors.New(ErrCodeSubscriptionInactive, "billing subscription is not active", http.StatusForbidden)
}

func SubscriptionSuspendedError() error {
	return coreerrors.New(ErrCodeSubscriptionSuspended, "billing subscription is suspended", http.StatusForbidden)
}

func FeatureNotEnabledError() error {
	return coreerrors.New(ErrCodeFeatureNotEnabled, "billing feature is not enabled", http.StatusForbidden)
}

func QuotaExceededError() error {
	return coreerrors.New(ErrCodeQuotaExceeded, "billing quota has been exceeded", http.StatusConflict)
}

func InvoiceNotFoundError() error {
	return coreerrors.New(ErrCodeInvoiceNotFound, "billing invoice not found", http.StatusNotFound)
}

func PaymentAlreadyProcessedError() error {
	return coreerrors.New(ErrCodePaymentAlreadyProcessed, "billing payment is already processed", http.StatusConflict)
}

func PaymentEventAlreadyProcessedError() error {
	return coreerrors.New(ErrCodePaymentEventAlreadyProcessed, "billing payment event is already processed", http.StatusConflict)
}

func InvalidBillingStatusError() error {
	return coreerrors.New(ErrCodeInvalidBillingStatus, "billing status is invalid", http.StatusBadRequest)
}
