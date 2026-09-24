package whatsapp

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
)

// ErrSessionNotFound is returned for a session id that does not exist in the
// caller's organization (or was soft-deleted).
var ErrSessionNotFound = coreerrors.New("WHATSAPP_SESSION_NOT_FOUND", "WhatsApp session not found", http.StatusNotFound)

// MapSessionNotFound translates the pgx.ErrNoRows every whatsapp repository
// returns for a scoped lookup/mutation that affects zero rows into
// ErrSessionNotFound. Other errors pass through unchanged.
func MapSessionNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSessionNotFound
	}
	return err
}
