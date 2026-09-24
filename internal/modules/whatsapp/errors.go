package whatsapp

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
)

var (
	// ErrSessionNotFound is returned for a session id that does not exist in
	// the caller's organization (or was soft-deleted).
	ErrSessionNotFound = coreerrors.New("WHATSAPP_SESSION_NOT_FOUND", "WhatsApp session not found", http.StatusNotFound)
	// ErrNotConfigured means the WAHA provider, webhook URL, or webhook HMAC
	// key is missing on the server; nothing the tenant can fix.
	ErrNotConfigured = coreerrors.New("WHATSAPP_NOT_CONFIGURED", "WhatsApp provider is not configured", http.StatusServiceUnavailable)
	// ErrProviderUnavailable hides WAHA error details (they may mention
	// server internals) behind a stable code.
	ErrProviderUnavailable = coreerrors.New("WHATSAPP_PROVIDER_ERROR", "WhatsApp provider request failed, please try again", http.StatusBadGateway)
	// ErrRemoteSessionMissing means WAHA no longer knows the session (deleted
	// on the server). The tenant should delete and recreate it.
	ErrRemoteSessionMissing = coreerrors.New("WHATSAPP_REMOTE_SESSION_MISSING", "WhatsApp session no longer exists on the provider; delete and reconnect it", http.StatusConflict)
	ErrSessionNotScanning   = coreerrors.New("WHATSAPP_SESSION_NOT_SCANNING", "WhatsApp session is not waiting for pairing; start it first", http.StatusConflict)
	ErrInvalidPairingPhone  = coreerrors.New("VALIDATION_ERROR", "phone must be a valid WhatsApp number, e.g. 0812xxxxxxx or +62812xxxxxxx", http.StatusUnprocessableEntity)
	ErrInvalidPurpose       = coreerrors.New("VALIDATION_ERROR", "purpose must be one of cs, sales, notification", http.StatusUnprocessableEntity)
)

// MapSessionNotFound translates the pgx.ErrNoRows every whatsapp repository
// returns for a scoped lookup/mutation that affects zero rows into
// ErrSessionNotFound. Other errors pass through unchanged.
func MapSessionNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSessionNotFound
	}
	return err
}

// MapProviderError converts platform WAHA errors into API errors. The
// original error is kept (Wrap) for server logs but never shown to clients.
func MapProviderError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, platformwhatsapp.ErrNotConfigured):
		return ErrNotConfigured
	case errors.Is(err, platformwhatsapp.ErrSessionNotFound):
		return coreerrors.Wrap(ErrRemoteSessionMissing.Code, ErrRemoteSessionMissing.Message, ErrRemoteSessionMissing.Status, err)
	default:
		return coreerrors.Wrap(ErrProviderUnavailable.Code, ErrProviderUnavailable.Message, ErrProviderUnavailable.Status, err)
	}
}
