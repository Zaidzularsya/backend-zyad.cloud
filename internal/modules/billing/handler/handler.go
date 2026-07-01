package handler

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
)

func validationHandlerError(err error) error {
	return coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity)
}
