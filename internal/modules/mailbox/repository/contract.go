package repository

import (
	"context"
	"errors"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mailbox/domain"
)

// ErrDuplicateMailbox: the user already connected this email address.
var ErrDuplicateMailbox = errors.New("mailbox already connected")

type MailboxParams struct {
	EmailAddress string
	DisplayName  string
	Username     string
	// SecretEncrypted empty on update keeps the stored password.
	SecretEncrypted string
	SMTPHost        string
	SMTPPort        int
	SMTPSecurity    domain.Security
	IMAPHost        string
	IMAPPort        int
	IMAPSecurity    domain.Security
}

// MailboxRepository always filters by user_id: a mailbox is only visible to
// the user who connected it.
type MailboxRepository interface {
	ListByUser(ctx context.Context, scope coretenant.Scope, userID string) ([]domain.Mailbox, error)
	// GetForUser returns pgx.ErrNoRows for another user's mailbox.
	GetForUser(ctx context.Context, scope coretenant.Scope, userID, id string) (domain.Mailbox, error)
	Create(ctx context.Context, scope coretenant.Scope, userID string, params MailboxParams) (domain.Mailbox, error)
	Update(ctx context.Context, scope coretenant.Scope, userID, id string, params MailboxParams) (domain.Mailbox, error)
	SetStatus(ctx context.Context, scope coretenant.Scope, id string, status domain.MailboxStatus, lastError string) error
	Delete(ctx context.Context, scope coretenant.Scope, userID, id string) error
}

type CreateOutboundParams struct {
	MailboxID         string
	ClientRequestID   string
	MessageID         string
	InReplyTo         string
	References        string
	FromAddress       string
	FromName          string
	To                []string
	Cc                []string
	Bcc               []string
	Subject           string
	Snippet           string
	BodyHTML          string
	BodyText          string
	RelatedEntityType domain.RelatedEntityType
	RelatedEntityID   string
	CreatedBy         string
}

type MessageListFilter struct {
	// UserID limits results to that user's mailboxes (required).
	UserID    string
	MailboxID string
	// Participant matches any from/to/cc/bcc address (case-insensitive).
	Participant       string
	Direction         domain.Direction
	RelatedEntityType domain.RelatedEntityType
	RelatedEntityID   string
	Limit             int
	Offset            int
}

type MessageRepository interface {
	// CreateOutbound inserts a queued message. A second call with the same
	// (mailbox, client request id) returns the existing row and created=false.
	CreateOutbound(ctx context.Context, scope coretenant.Scope, params CreateOutboundParams) (message domain.Message, created bool, err error)
	AddAttachment(ctx context.Context, scope coretenant.Scope, messageID, assetObjectID string) error
	MarkSent(ctx context.Context, scope coretenant.Scope, id string, sentAt time.Time) error
	MarkFailed(ctx context.Context, scope coretenant.Scope, id, errorMessage string) error
	// FailStaleQueued marks the user's messages still queued before cutoff
	// as failed (the sender stopped, e.g. on a restart).
	FailStaleQueued(ctx context.Context, scope coretenant.Scope, userID string, cutoff time.Time) error
	List(ctx context.Context, scope coretenant.Scope, filter MessageListFilter) ([]domain.Message, int64, error)
	// GetForUser loads a message with its attachments; pgx.ErrNoRows when it
	// is not in one of the user's mailboxes.
	GetForUser(ctx context.Context, scope coretenant.Scope, userID, id string) (domain.Message, error)
	// AttachmentAssetIDs lists the asset objects of all messages of a
	// mailbox, so they can be removed from storage before the mailbox.
	AttachmentAssetIDs(ctx context.Context, scope coretenant.Scope, mailboxID string) ([]string, error)
}
