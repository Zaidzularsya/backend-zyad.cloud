package service

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	billing "zyad.cloud/internal/modules/billing"
)

func validationError(message string) error {
	return coreerrors.New("VALIDATION_ERROR", message, http.StatusUnprocessableEntity)
}

func mapPlanError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.PlanNotFoundError()
	}
	return err
}

func mapPlanPriceError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.PlanPriceNotFoundError()
	}
	return err
}

func mapFeatureError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.FeatureNotFoundError()
	}
	return err
}

func mapSubscriptionError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.SubscriptionNotFoundError()
	}
	return err
}

func mapInvoiceError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.InvoiceNotFoundError()
	}
	return err
}
