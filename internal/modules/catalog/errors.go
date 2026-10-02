package catalog

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
)

func MapNotFound(err error, code, message string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return coreerrors.New(code, message, http.StatusNotFound)
	}
	return err
}
