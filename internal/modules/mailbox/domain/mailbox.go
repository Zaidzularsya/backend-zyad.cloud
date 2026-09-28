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
	SMTPHost        string
	SMTPPort        int
	SMTPSecurity    Security
	IMAPHost        string
	IMAPPort        int
	IMAPSecurity    Security
	Status          MailboxStatus
	LastError       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
