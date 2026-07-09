package subscription

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
)

const (
	ErrCodeSubscriptionNotFound  = "SUBSCRIPTION_NOT_FOUND"
	ErrCodeSubscriptionInactive  = "SUBSCRIPTION_INACTIVE"
	ErrCodeSubscriptionSuspended = "SUBSCRIPTION_SUSPENDED"
	ErrCodeFeatureNotEnabled     = "FEATURE_NOT_ENABLED"
	ErrCodeQuotaExceeded         = "QUOTA_EXCEEDED"
	ErrCodeInvalidBillingStatus  = "INVALID_BILLING_STATUS"
)

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

func InvalidBillingStatusError() error {
	return coreerrors.New(ErrCodeInvalidBillingStatus, "billing status is invalid", http.StatusBadRequest)
}
