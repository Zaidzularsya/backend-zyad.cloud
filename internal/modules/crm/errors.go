package crm

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
)

// MapNotFound translates a pgx.ErrNoRows (the sentinel every crm repository
// returns when a scoped id lookup/mutation affects zero rows) into a proper
// 404 AppError, mirroring mapPagePersistenceError in the landing module.
// Any other error is passed through unchanged.
func MapNotFound(err error, code string, message string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return coreerrors.New(code, message, http.StatusNotFound)
	}
	return err
}
