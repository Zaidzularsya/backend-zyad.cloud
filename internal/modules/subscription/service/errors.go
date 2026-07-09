package service

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	subscription "zyad.cloud/internal/modules/subscription"
)

func validationError(message string) error {
	return coreerrors.New("VALIDATION_ERROR", message, http.StatusUnprocessableEntity)
}

func mapSubscriptionError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return subscription.SubscriptionNotFoundError()
	}
	return err
}
