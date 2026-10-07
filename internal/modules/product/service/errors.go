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

func mapFeatureError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return product.FeatureNotFoundError()
	}
	return err
}
