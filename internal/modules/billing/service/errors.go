package service

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	billing "zyad.cloud/internal/modules/billing"
	subscription "zyad.cloud/internal/modules/subscription"
)

func validationError(message string) error {
	return coreerrors.New("VALIDATION_ERROR", message, http.StatusUnprocessableEntity)
}

func mapInvoiceError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.InvoiceNotFoundError()
	}
	return err
}

// mapSubscriptionError translates a not-found lookup against the
// subscription domain (accessed here only through its public dto-shaped
// interfaces) into the shared SUBSCRIPTION_NOT_FOUND error code.
func mapSubscriptionError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return subscription.SubscriptionNotFoundError()
	}
	return err
}

// isSubscriptionNotFound matches both the raw row-level pgx.ErrNoRows and the
// subscription domain's mapped SUBSCRIPTION_NOT_FOUND error, since
// FindLatestByOrganization surfaces the latter for empty result sets.
func isSubscriptionNotFound(err error) bool {
	if errors.Is(err, pgx.ErrNoRows) {
		return true
	}
	var appErr *coreerrors.AppError
	return errors.As(err, &appErr) && appErr.Code == subscription.ErrCodeSubscriptionNotFound
}
