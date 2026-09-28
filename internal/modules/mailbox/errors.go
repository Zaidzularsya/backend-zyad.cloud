package mailbox

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
)

var (
	// ErrMailboxNotFound is also returned for another user's mailbox, so its
	// existence is not revealed.
	ErrMailboxNotFound   = coreerrors.New("MAILBOX_NOT_FOUND", "mailbox not found", http.StatusNotFound)
	ErrMailboxDuplicate  = coreerrors.New("MAILBOX_DUPLICATE", "this email address is already connected", http.StatusConflict)
	ErrMailboxInactive   = coreerrors.New("MAILBOX_INACTIVE", "mailbox is disabled; enable it or connect it again", http.StatusConflict)
	ErrMessageNotFound   = coreerrors.New("EMAIL_MESSAGE_NOT_FOUND", "email not found", http.StatusNotFound)
	ErrAttachmentMissing = coreerrors.New("EMAIL_ATTACHMENT_NOT_FOUND", "attachment not found", http.StatusNotFound)
	ErrEntityNotFound    = coreerrors.New("EMAIL_ENTITY_NOT_FOUND", "lead or contact not found", http.StatusNotFound)
	// ErrHostNotAllowed hides whether the host exists; it only says the
	// address is not a public mail server.
	ErrHostNotAllowed      = coreerrors.New("MAILBOX_HOST_NOT_ALLOWED", "mail server must be a public host", http.StatusUnprocessableEntity)
	ErrAttachmentsTooLarge = coreerrors.New("VALIDATION_ERROR", "total attachment size must be at most 18MB", http.StatusUnprocessableEntity)
	ErrAttachmentType      = coreerrors.New("VALIDATION_ERROR", "attachment type is not allowed (PDF, images, Office, CSV, TXT, ZIP)", http.StatusUnprocessableEntity)
)

// ConnectionError reports why SMTP verification failed. The provider
// message is kept (e.g. "535 authentication failed") because the user has to
// fix their own settings.
func ConnectionError(detail string) error {
	return coreerrors.New("MAILBOX_CONNECTION_FAILED", "could not connect to the mail server: "+detail, http.StatusUnprocessableEntity)
}
