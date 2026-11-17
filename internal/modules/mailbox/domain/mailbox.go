package domain

import "time"

type Security string

const (
	// SecuritySSL is implicit TLS (SMTP 465 / IMAP 993).
	SecuritySSL Security = "ssl"
	// SecuritySTARTTLS upgrades a plain connection (SMTP 587); the server
	// must support it, plaintext is never used.
	SecuritySTARTTLS Security = "starttls"
)

func (s Security) IsValid() bool {
	return s == SecuritySSL || s == SecuritySTARTTLS
}

// AuthMethod is how the mailbox logs in to SMTP/IMAP.
type AuthMethod string

const (
	// AuthMethodPassword: SecretEncrypted is the (app) password.
	AuthMethodPassword AuthMethod = "password"
	// AuthMethodMicrosoftOAuth: SecretEncrypted is a Microsoft identity
	// platform refresh token; SMTP/IMAP use SASL XOAUTH2 with an access
	// token derived from it (cached in AccessTokenEncrypted).
	AuthMethodMicrosoftOAuth AuthMethod = "microsoft_oauth"
)

func (a AuthMethod) IsOAuth() bool {
	return a == AuthMethodMicrosoftOAuth
}

type MailboxStatus string

const (
	MailboxStatusActive   MailboxStatus = "active"
	MailboxStatusError    MailboxStatus = "error"
	MailboxStatusDisabled MailboxStatus = "disabled"
)

// Mailbox is one email account a user connected. SecretEncrypted is only
// read by the service to send; it never leaves the backend.
type Mailbox struct {
	ID              string
	UserID          string
	EmailAddress    string
	DisplayName     string
	Username        string
	SecretEncrypted string
	AuthMethod      AuthMethod
	// AccessTokenEncrypted/AccessTokenExpiresAt cache the OAuth access
	// token; empty/nil for password mailboxes.
	AccessTokenEncrypted string
	AccessTokenExpiresAt *time.Time
	SMTPHost             string
	SMTPPort             int
	SMTPSecurity         Security
	IMAPHost             string
	IMAPPort             int
	IMAPSecurity         Security
	Status               MailboxStatus
	LastError            string
	SyncCursor           SyncCursor
	LastSyncedAt         *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// HasIMAP reports whether inbox sync can run for this mailbox.
func (m Mailbox) HasIMAP() bool {
	return m.IMAPHost != "" && m.IMAPPort != 0 && m.IMAPSecurity.IsValid()
}
