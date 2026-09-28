package service

import (
	"context"
	"time"

	"zyad.cloud/internal/modules/mailbox/domain"
)

// FetchedAttachment is one attachment of a synced message, still in memory.
type FetchedAttachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// FetchedMessage is one message read from INBOX over IMAP, already parsed.
type FetchedMessage struct {
	UID         uint32
	MessageID   string
	InReplyTo   string
	FromAddress string
	FromName    string
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Date        time.Time
	BodyHTML    string
	BodyText    string
	Attachments []FetchedAttachment
}

// IMAPConn is an authenticated connection with INBOX selected.
type IMAPConn interface {
	// UIDValidity of the selected folder; changing since the stored cursor
	// means the folder was recreated and old UIDs no longer apply.
	UIDValidity() uint32
	// FetchSince returns messages with UID > afterUID, oldest first,
	// capped at limit. When afterUID is 0 (first sync of this mailbox),
	// only messages received on/after cutoff are considered.
	FetchSince(ctx context.Context, afterUID uint32, cutoff time.Time, limit int) ([]FetchedMessage, error)
	Close() error
}

// IMAPDialConfig is what dialing needs from a mailbox; kept separate from
// domain.Mailbox so the dialer doesn't depend on the whole struct (or the
// encrypted secret — the caller decrypts it once).
type IMAPDialConfig struct {
	Host     string
	Port     int
	Security domain.Security
	Username string
	Password string
}

// IMAPDialer connects and authenticates, and selects INBOX.
type IMAPDialer func(ctx context.Context, cfg IMAPDialConfig, allowPrivateHosts bool) (IMAPConn, error)
