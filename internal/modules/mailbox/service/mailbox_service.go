package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	corecrypto "zyad.cloud/internal/core/crypto"
	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	mailboxmodule "zyad.cloud/internal/modules/mailbox"
	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/mailbox/repository"
	platformmail "zyad.cloud/internal/platform/mail"
)

// verifyTimeout stays below the HTTP write timeout (15s) so a slow mail
// server produces a readable error instead of a dropped connection.
const verifyTimeout = 12 * time.Second

type MailboxInput struct {
	EmailAddress string
	DisplayName  string
	Username     string
	// Password is required on create; empty on update keeps the stored one.
	Password     string
	SMTPHost     string
	SMTPPort     int
	SMTPSecurity domain.Security
	IMAPHost     string
	IMAPPort     int
	IMAPSecurity domain.Security
}

// AttachmentStorage is the subset of assetservice.AssetService this module
// uses. Attachments are ordinary private tenant objects (quota included).
type AttachmentStorage interface {
	UploadObject(ctx context.Context, scope coretenant.Scope, params assetservice.UploadObjectParams, content io.Reader) (assetdomain.AssetObject, error)
	DeleteObject(ctx context.Context, scope coretenant.Scope, id string) error
	DownloadURL(ctx context.Context, scope coretenant.Scope, id string) (assetservice.DownloadResult, error)
}

// SMTPVerifier checks that a mailbox's SMTP settings connect and log in.
type SMTPVerifier func(ctx context.Context, cfg platformmail.SMTPConfig) error

type MailboxServiceOptions struct {
	// SecretKey derives the AES key of user_mailboxes.secret_encrypted.
	SecretKey string
	// AllowPrivateHosts disables the public-address check (development
	// against a local mail catcher only).
	AllowPrivateHosts bool
	Verify            SMTPVerifier
}

type MailboxService struct {
	mailboxes repository.MailboxRepository
	messages  repository.MessageRepository
	storage   AttachmentStorage
	secretKey string
	verify    SMTPVerifier
	allowAny  bool
}

func NewMailboxService(
	mailboxes repository.MailboxRepository,
	messages repository.MessageRepository,
	storage AttachmentStorage,
	options MailboxServiceOptions,
) *MailboxService {
	verify := options.Verify
	if verify == nil {
		verify = platformmail.VerifySMTP
	}
	return &MailboxService{
		mailboxes: mailboxes,
		messages:  messages,
		storage:   storage,
		secretKey: options.SecretKey,
		verify:    verify,
		allowAny:  options.AllowPrivateHosts,
	}
}

func (s *MailboxService) List(ctx context.Context, scope coretenant.Scope, userID string) ([]domain.Mailbox, error) {
	return s.mailboxes.ListByUser(ctx, scope, userID)
}

// Create verifies the SMTP settings before storing them, so a mailbox in
// the list is known to work.
func (s *MailboxService) Create(ctx context.Context, scope coretenant.Scope, userID string, input MailboxInput) (domain.Mailbox, error) {
	input = normalizeMailboxInput(input)
	if strings.TrimSpace(input.Password) == "" {
		return domain.Mailbox{}, validationError("password is required")
	}
	if err := validateMailboxInput(input); err != nil {
		return domain.Mailbox{}, err
	}
	if err := s.checkConnection(ctx, input, input.Password); err != nil {
		return domain.Mailbox{}, err
	}
	encrypted, err := corecrypto.EncryptSecret(s.secretKey, input.Password)
	if err != nil {
		return domain.Mailbox{}, err
	}
	mailbox, err := s.mailboxes.Create(ctx, scope, userID, toParams(input, encrypted))
	if errors.Is(err, repository.ErrDuplicateMailbox) {
		return domain.Mailbox{}, mailboxmodule.ErrMailboxDuplicate
	}
	return mailbox, err
}

func (s *MailboxService) Update(ctx context.Context, scope coretenant.Scope, userID, id string, input MailboxInput) (domain.Mailbox, error) {
	current, err := s.get(ctx, scope, userID, id)
	if err != nil {
		return domain.Mailbox{}, err
	}
	input = normalizeMailboxInput(input)
	if err := validateMailboxInput(input); err != nil {
		return domain.Mailbox{}, err
	}

	password := input.Password
	encrypted := ""
	if strings.TrimSpace(password) == "" {
		if password, err = corecrypto.DecryptSecret(s.secretKey, current.SecretEncrypted); err != nil {
			return domain.Mailbox{}, err
		}
	} else if encrypted, err = corecrypto.EncryptSecret(s.secretKey, password); err != nil {
		return domain.Mailbox{}, err
	}
	if err := s.checkConnection(ctx, input, password); err != nil {
		return domain.Mailbox{}, err
	}

	mailbox, err := s.mailboxes.Update(ctx, scope, userID, id, toParams(input, encrypted))
	if errors.Is(err, repository.ErrDuplicateMailbox) {
		return domain.Mailbox{}, mailboxmodule.ErrMailboxDuplicate
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Mailbox{}, mailboxmodule.ErrMailboxNotFound
	}
	return mailbox, err
}

// Test re-checks a stored mailbox and records the outcome in its status.
func (s *MailboxService) Test(ctx context.Context, scope coretenant.Scope, userID, id string) (domain.Mailbox, error) {
	mailbox, err := s.get(ctx, scope, userID, id)
	if err != nil {
		return domain.Mailbox{}, err
	}
	password, err := corecrypto.DecryptSecret(s.secretKey, mailbox.SecretEncrypted)
	if err != nil {
		return domain.Mailbox{}, err
	}
	checkErr := s.checkConnection(ctx, inputFromMailbox(mailbox), password)
	status, lastError := domain.MailboxStatusActive, ""
	if checkErr != nil {
		status, lastError = domain.MailboxStatusError, checkErr.Error()
	}
	if err := s.mailboxes.SetStatus(ctx, scope, mailbox.ID, status, lastError); err != nil {
		return domain.Mailbox{}, err
	}
	mailbox.Status, mailbox.LastError = status, lastError
	return mailbox, checkErr
}

// Delete removes the mailbox's attachment files from tenant storage first;
// the rows (messages, attachment links) cascade with the mailbox.
func (s *MailboxService) Delete(ctx context.Context, scope coretenant.Scope, userID, id string) error {
	if _, err := s.get(ctx, scope, userID, id); err != nil {
		return err
	}
	assetIDs, err := s.messages.AttachmentAssetIDs(ctx, scope, id)
	if err != nil {
		return err
	}
	for _, assetID := range assetIDs {
		if err := s.storage.DeleteObject(ctx, scope, assetID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	if err := s.mailboxes.Delete(ctx, scope, userID, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return mailboxmodule.ErrMailboxNotFound
		}
		return err
	}
	return nil
}

// Get returns a mailbox the caller owns, or ErrMailboxNotFound. Used by the
// handler to validate a mailbox id before starting a background action
// (sync now) that has no other way to report "not found" to the request.
func (s *MailboxService) Get(ctx context.Context, scope coretenant.Scope, userID, id string) (domain.Mailbox, error) {
	return s.get(ctx, scope, userID, id)
}

func (s *MailboxService) get(ctx context.Context, scope coretenant.Scope, userID, id string) (domain.Mailbox, error) {
	mailbox, err := s.mailboxes.GetForUser(ctx, scope, userID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Mailbox{}, mailboxmodule.ErrMailboxNotFound
	}
	return mailbox, err
}

func (s *MailboxService) checkConnection(ctx context.Context, input MailboxInput, password string) error {
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	cfg := smtpConfig(input.EmailAddress, input.DisplayName, input.Username, password,
		input.SMTPHost, input.SMTPPort, input.SMTPSecurity, s.allowAny)
	if err := s.verify(ctx, cfg); err != nil {
		if errors.Is(err, mailboxmodule.ErrHostNotAllowed) {
			return mailboxmodule.ErrHostNotAllowed
		}
		return mailboxmodule.ConnectionError(err.Error())
	}
	return nil
}

// smtpConfig never allows plaintext: SSL is implicit TLS, STARTTLS is
// required (RequireTLS) rather than best effort.
func smtpConfig(email, displayName, username, password, host string, port int, security domain.Security, allowPrivate bool) platformmail.SMTPConfig {
	from := email
	if displayName != "" {
		from = (&netmail.Address{Name: displayName, Address: email}).String()
	}
	return platformmail.SMTPConfig{
		Host:        host,
		Port:        port,
		User:        username,
		Password:    password,
		From:        from,
		Secure:      security == domain.SecuritySSL,
		TLS:         security == domain.SecuritySTARTTLS,
		RequireTLS:  true,
		DialControl: publicDialControl(allowPrivate),
	}
}

func normalizeMailboxInput(input MailboxInput) MailboxInput {
	input.EmailAddress = strings.ToLower(strings.TrimSpace(input.EmailAddress))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" {
		input.Username = input.EmailAddress
	}
	input.SMTPHost = strings.ToLower(strings.TrimSpace(input.SMTPHost))
	input.IMAPHost = strings.ToLower(strings.TrimSpace(input.IMAPHost))
	return input
}

func validateMailboxInput(input MailboxInput) error {
	if address, err := netmail.ParseAddress(input.EmailAddress); err != nil || address.Address != input.EmailAddress {
		return validationError("email_address must be a valid email address")
	}
	if len(input.DisplayName) > 150 {
		return validationError("display_name must be at most 150 characters")
	}
	if input.SMTPHost == "" || strings.ContainsAny(input.SMTPHost, " /:") {
		return validationError("smtp_host must be a host name")
	}
	if !allowedSMTPPorts[input.SMTPPort] {
		return validationError("smtp_port must be one of 25, 465, 587, 2525")
	}
	if !input.SMTPSecurity.IsValid() {
		return validationError("smtp_security must be ssl or starttls")
	}
	// IMAP is optional until inbox sync is enabled; when given it must be
	// complete.
	if input.IMAPHost != "" || input.IMAPPort != 0 || input.IMAPSecurity != "" {
		if input.IMAPHost == "" || strings.ContainsAny(input.IMAPHost, " /:") {
			return validationError("imap_host must be a host name")
		}
		if !allowedIMAPPorts[input.IMAPPort] {
			return validationError("imap_port must be 993 or 143")
		}
		if !input.IMAPSecurity.IsValid() {
			return validationError("imap_security must be ssl or starttls")
		}
	}
	return nil
}

func toParams(input MailboxInput, encrypted string) repository.MailboxParams {
	return repository.MailboxParams{
		EmailAddress:    input.EmailAddress,
		DisplayName:     input.DisplayName,
		Username:        input.Username,
		SecretEncrypted: encrypted,
		SMTPHost:        input.SMTPHost,
		SMTPPort:        input.SMTPPort,
		SMTPSecurity:    input.SMTPSecurity,
		IMAPHost:        input.IMAPHost,
		IMAPPort:        input.IMAPPort,
		IMAPSecurity:    input.IMAPSecurity,
	}
}

func inputFromMailbox(m domain.Mailbox) MailboxInput {
	return MailboxInput{
		EmailAddress: m.EmailAddress, DisplayName: m.DisplayName, Username: m.Username,
		SMTPHost: m.SMTPHost, SMTPPort: m.SMTPPort, SMTPSecurity: m.SMTPSecurity,
		IMAPHost: m.IMAPHost, IMAPPort: m.IMAPPort, IMAPSecurity: m.IMAPSecurity,
	}
}

func validationError(message string) error {
	return coreerrors.New("VALIDATION_ERROR", message, http.StatusUnprocessableEntity)
}
