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
	ErrIMAPNotConfigured   = coreerrors.New("MAILBOX_IMAP_NOT_CONFIGURED", "IMAP settings are not set for this mailbox", http.StatusUnprocessableEntity)
	// ErrSyncFailed hides the raw IMAP error from the request (it may name
	// internal hosts); the mailbox's status/last_error carries the detail
	// for the account owner to read once it's saved.
	ErrSyncFailed = coreerrors.New("MAILBOX_SYNC_FAILED", "inbox sync failed, check the mailbox's connection status", http.StatusUnprocessableEntity)
)

var (
	ErrOAuthNotConfigured = coreerrors.New("MAILBOX_OAUTH_NOT_CONFIGURED", "Microsoft sign-in is not configured on this server", http.StatusServiceUnavailable)
	// ErrOAuthStateInvalid: the callback's state is expired, tampered, or
	// was issued to another user/organization.
	ErrOAuthStateInvalid = coreerrors.New("MAILBOX_OAUTH_STATE_INVALID", "the Microsoft sign-in link expired or is not yours; start again", http.StatusUnprocessableEntity)
)

// OAuthFailed reports a failed code exchange (Microsoft's error code and
// description, e.g. AADSTS65001 consent required).
func OAuthFailed(detail string) error {
	return coreerrors.New("MAILBOX_OAUTH_FAILED", "Microsoft sign-in failed: "+detail, http.StatusUnprocessableEntity)
}

// OAuthAccountMismatch: a reconnect signed in with a different account.
func OAuthAccountMismatch(expected, got string) error {
	return coreerrors.New("MAILBOX_OAUTH_ACCOUNT_MISMATCH",
		"signed in as "+got+", but this mailbox is "+expected+"; sign in with "+expected+" or connect it as a new mailbox",
		http.StatusConflict)
}

// ConnectionError reports why SMTP verification failed. The provider
// message is kept (e.g. "535 authentication failed") because the user has to
// fix their own settings.
func ConnectionError(detail string) error {
	return coreerrors.New("MAILBOX_CONNECTION_FAILED", "could not connect to the mail server: "+detail, http.StatusUnprocessableEntity)
}
