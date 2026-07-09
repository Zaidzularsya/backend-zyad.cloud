package service

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	product "zyad.cloud/internal/modules/product"
)

func validationError(message string) error {
	return coreerrors.New("VALIDATION_ERROR", message, http.StatusUnprocessableEntity)
}

func mapPlanError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return product.PlanNotFoundError()
	}
	return err
}

func mapPlanPriceError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return product.PlanPriceNotFoundError()
	}
	return err
}

func mapFeatureError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return product.FeatureNotFoundError()
	}
	return err
}
