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

	// ErrConversationNotFound is also returned for conversations the viewer
	// may not read, so their existence is not revealed.
	ErrConversationNotFound  = coreerrors.New("WHATSAPP_CONVERSATION_NOT_FOUND", "WhatsApp conversation not found", http.StatusNotFound)
	ErrConversationForbidden = coreerrors.New("WHATSAPP_CONVERSATION_FORBIDDEN", "this WhatsApp conversation is assigned to another user", http.StatusForbidden)
	ErrMessageNotFound       = coreerrors.New("WHATSAPP_MESSAGE_NOT_FOUND", "WhatsApp message not found", http.StatusNotFound)
	ErrMessageNotRetryable   = coreerrors.New("WHATSAPP_MESSAGE_NOT_RETRYABLE", "only failed outgoing messages can be retried", http.StatusConflict)
	ErrSessionNotConnected   = coreerrors.New("WHATSAPP_SESSION_NOT_CONNECTED", "WhatsApp session is not connected; reconnect it first", http.StatusConflict)
	ErrNoConnectedSession    = coreerrors.New("WHATSAPP_NO_CONNECTED_SESSION", "no connected WhatsApp session; connect a number first", http.StatusConflict)
	ErrEntityNotFound        = coreerrors.New("WHATSAPP_ENTITY_NOT_FOUND", "lead or contact not found", http.StatusNotFound)
	ErrEntityPhoneInvalid    = coreerrors.New("WHATSAPP_ENTITY_PHONE_INVALID", "the lead or contact has no valid phone number", http.StatusUnprocessableEntity)
	ErrRateLimited           = coreerrors.New("WHATSAPP_RATE_LIMITED", "too many WhatsApp messages from this number; wait a minute and try again", http.StatusTooManyRequests)
	ErrInvalidMessageText    = coreerrors.New("VALIDATION_ERROR", "text is required and must be at most 4096 characters", http.StatusUnprocessableEntity)
	ErrInvalidEntityType     = coreerrors.New("VALIDATION_ERROR", "related_entity_type must be lead or contact", http.StatusUnprocessableEntity)
	ErrInvalidAssignee       = coreerrors.New("VALIDATION_ERROR", "assignee must be an active member of the organization", http.StatusUnprocessableEntity)
	ErrInvalidStatus         = coreerrors.New("VALIDATION_ERROR", "status must be open or closed", http.StatusUnprocessableEntity)
	ErrAssignForbidden       = coreerrors.New("FORBIDDEN", "reassigning a conversation requires whatsapp.conversation.assign", http.StatusForbidden)
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
