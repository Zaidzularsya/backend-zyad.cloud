package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	corecrypto "zyad.cloud/internal/core/crypto"
	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	mailboxmodule "zyad.cloud/internal/modules/mailbox"
	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/mailbox/repository"
	platformmail "zyad.cloud/internal/platform/mail"
)

const (
	maxRecipients = 50
	// deliveryTimeout bounds one SMTP delivery running in the background.
	deliveryTimeout = 2 * time.Minute
	// staleQueuedAfter: a message still queued this long lost its sender
	// (process restart) and is reported as failed.
	staleQueuedAfter = 10 * time.Minute
	activityBodyMax  = 2000
)

// EnvelopeSender delivers a raw MIME message (platformmail.SendEnvelope).
type EnvelopeSender func(ctx context.Context, cfg platformmail.SMTPConfig, from string, recipients []string, raw []byte) error

type FileInput struct {
	Filename string
	Size     int64
	Content  io.Reader
}

type SendInput struct {
	// ClientRequestID makes a retried submit (double click, network retry)
	// return the first message instead of sending again.
	ClientRequestID   string
	To                []string
	Cc                []string
	Bcc               []string
	Subject           string
	BodyHTML          string
	InReplyTo         string
	RelatedEntityType domain.RelatedEntityType
	RelatedEntityID   string
	Files             []FileInput
}

type MessageListInput struct {
	MailboxID         string
	Participant       string
	Direction         domain.Direction
	RelatedEntityType domain.RelatedEntityType
	RelatedEntityID   string
	Page              int
	PerPage           int
}

type MessageServiceOptions struct {
	SecretKey         string
	AllowPrivateHosts bool
	Send              EnvelopeSender
	Log               *slog.Logger
	// Synchronous delivers inside Send instead of in a goroutine (tests).
	Synchronous bool
}

type MessageService struct {
	mailboxes   repository.MailboxRepository
	messages    repository.MessageRepository
	storage     AttachmentStorage
	crm         CRMEntities
	secretKey   string
	allowAny    bool
	send        EnvelopeSender
	log         *slog.Logger
	synchronous bool
	now         func() time.Time
}

func NewMessageService(
	mailboxes repository.MailboxRepository,
	messages repository.MessageRepository,
	storage AttachmentStorage,
	crm CRMEntities,
	options MessageServiceOptions,
) *MessageService {
	send := options.Send
	if send == nil {
		send = platformmail.SendEnvelope
	}
	log := options.Log
	if log == nil {
		log = slog.Default()
	}
	return &MessageService{
		mailboxes:   mailboxes,
		messages:    messages,
		storage:     storage,
		crm:         crm,
		secretKey:   options.SecretKey,
		allowAny:    options.AllowPrivateHosts,
		send:        send,
		log:         log,
		synchronous: options.Synchronous,
		now:         time.Now,
	}
}

type preparedFile struct {
	filename string
	mimeType string
	content  []byte
}

// Send stores the message as queued (with its attachments in tenant
// storage) and delivers it over SMTP in the background, so a slow mail
// server does not hit the HTTP timeout. The caller polls the message status.
func (s *MessageService) Send(ctx context.Context, scope coretenant.Scope, userID, mailboxID string, input SendInput) (domain.Message, error) {
	mailbox, err := s.mailboxes.GetForUser(ctx, scope, userID, mailboxID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Message{}, mailboxmodule.ErrMailboxNotFound
	}
	if err != nil {
		return domain.Message{}, err
	}
	if mailbox.Status == domain.MailboxStatusDisabled {
		return domain.Message{}, mailboxmodule.ErrMailboxInactive
	}

	to, cc, bcc, err := validateRecipients(input.To, input.Cc, input.Bcc)
	if err != nil {
		return domain.Message{}, err
	}
	subject := strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(input.Subject))
	if subject == "" {
		return domain.Message{}, validationError("subject is required")
	}
	if len(subject) > 998 {
		return domain.Message{}, validationError("subject is too long")
	}
	if len(input.ClientRequestID) > 100 {
		return domain.Message{}, validationError("client request id is too long")
	}
	if input.RelatedEntityType != "" || input.RelatedEntityID != "" {
		if !input.RelatedEntityType.IsValid() || input.RelatedEntityID == "" {
			return domain.Message{}, validationError("related_entity_type must be lead or contact, with related_entity_id")
		}
		if err := s.crm.RequireEntity(ctx, scope, input.RelatedEntityType, input.RelatedEntityID); err != nil {
			return domain.Message{}, err
		}
	}
	files, err := readFiles(input.Files)
	if err != nil {
		return domain.Message{}, err
	}

	bodyHTML := sanitizeHTML(input.BodyHTML)
	bodyText := htmlToText(bodyHTML)
	messageID, err := platformmail.NewMessageID(domainOf(mailbox.EmailAddress))
	if err != nil {
		return domain.Message{}, err
	}
	inReplyTo := strings.TrimSpace(input.InReplyTo)

	message, created, err := s.messages.CreateOutbound(ctx, scope, repository.CreateOutboundParams{
		MailboxID:         mailbox.ID,
		ClientRequestID:   strings.TrimSpace(input.ClientRequestID),
		MessageID:         messageID,
		InReplyTo:         inReplyTo,
		References:        inReplyTo,
		FromAddress:       mailbox.EmailAddress,
		FromName:          mailbox.DisplayName,
		To:                addressStrings(to),
		Cc:                addressStrings(cc),
		Bcc:               addressStrings(bcc),
		Subject:           subject,
		Snippet:           snippet(bodyText, 200),
		BodyHTML:          bodyHTML,
		BodyText:          bodyText,
		RelatedEntityType: input.RelatedEntityType,
		RelatedEntityID:   input.RelatedEntityID,
		CreatedBy:         userID,
	})
	if err != nil {
		return domain.Message{}, err
	}
	if !created {
		return s.messages.GetForUser(ctx, scope, userID, message.ID)
	}

	for _, file := range files {
		if err := s.storeAttachment(ctx, scope, userID, message.ID, file); err != nil {
			_ = s.messages.MarkFailed(ctx, scope, message.ID, "attachment upload failed")
			return domain.Message{}, err
		}
	}

	raw, err := platformmail.BuildMIME(platformmail.MIMEMessage{
		From:        netmail.Address{Name: mailbox.DisplayName, Address: mailbox.EmailAddress},
		To:          to,
		Cc:          cc,
		Subject:     subject,
		TextBody:    bodyText,
		HTMLBody:    bodyHTML,
		MessageID:   messageID,
		InReplyTo:   inReplyTo,
		References:  nonEmpty(inReplyTo),
		Date:        s.now(),
		Attachments: mimeAttachments(files),
	})
	if err != nil {
		_ = s.messages.MarkFailed(ctx, scope, message.ID, "could not build the message")
		return domain.Message{}, err
	}

	recipients := append(append(addressStrings(to), addressStrings(cc)...), addressStrings(bcc)...)
	deliver := func(ctx context.Context) {
		s.deliver(ctx, scope, userID, mailbox, message, recipients, raw)
	}
	if s.synchronous {
		deliver(ctx)
	} else {
		// Detached from the request: it must finish after the response.
		go deliver(context.WithoutCancel(ctx))
	}
	return s.messages.GetForUser(ctx, scope, userID, message.ID)
}

func (s *MessageService) deliver(ctx context.Context, scope coretenant.Scope, userID string, mailbox domain.Mailbox, message domain.Message, recipients []string, raw []byte) {
	// Only the SMTP exchange is bounded; recording the outcome must still
	// work after a delivery timeout.
	sendCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
	defer cancel()

	password, err := corecrypto.DecryptSecret(s.secretKey, mailbox.SecretEncrypted)
	if err == nil {
		cfg := smtpConfig(mailbox.EmailAddress, mailbox.DisplayName, mailbox.Username, password,
			mailbox.SMTPHost, mailbox.SMTPPort, mailbox.SMTPSecurity, s.allowAny)
		err = s.send(sendCtx, cfg, mailbox.EmailAddress, recipients, raw)
	}
	if err != nil {
		s.log.WarnContext(ctx, "email delivery failed", "message_id", message.ID, "mailbox_id", mailbox.ID, "error", err)
		if markErr := s.messages.MarkFailed(ctx, scope, message.ID, err.Error()); markErr != nil {
			s.log.ErrorContext(ctx, "mark email failed", "message_id", message.ID, "error", markErr)
		}
		return
	}

	if err := s.messages.MarkSent(ctx, scope, message.ID, s.now().UTC()); err != nil {
		s.log.ErrorContext(ctx, "mark email sent", "message_id", message.ID, "error", err)
	}
	if message.RelatedEntityType != "" {
		description := "To: " + strings.Join(message.To, ", ")
		if message.BodyText != "" {
			description += "\n\n" + truncate(message.BodyText, activityBodyMax)
		}
		if err := s.crm.RecordEmailActivity(ctx, scope, EmailActivityInput{
			EntityType:  message.RelatedEntityType,
			EntityID:    message.RelatedEntityID,
			Subject:     message.Subject,
			Description: description,
			UserID:      userID,
		}); err != nil {
			s.log.WarnContext(ctx, "record email activity", "message_id", message.ID, "error", err)
		}
	}
}

func (s *MessageService) storeAttachment(ctx context.Context, scope coretenant.Scope, userID, messageID string, file preparedFile) error {
	object, err := s.storage.UploadObject(ctx, scope, assetservice.UploadObjectParams{
		Filename:         file.filename,
		MimeType:         file.mimeType,
		SizeBytes:        int64(len(file.content)),
		Class:            assetdomain.ObjectClassPrivate,
		Label:            "mail_message:" + messageID,
		CreatedBy:        userID,
		AllowedMimeTypes: attachmentMimeTypes,
	}, bytes.NewReader(file.content))
	if err != nil {
		return err
	}
	if err := s.messages.AddAttachment(ctx, scope, messageID, object.ID); err != nil {
		_ = s.storage.DeleteObject(ctx, scope, object.ID)
		return err
	}
	return nil
}

func (s *MessageService) List(ctx context.Context, scope coretenant.Scope, userID string, input MessageListInput) ([]domain.Message, int64, error) {
	if err := s.messages.FailStaleQueued(ctx, scope, userID, s.now().UTC().Add(-staleQueuedAfter)); err != nil {
		return nil, 0, err
	}
	perPage := input.PerPage
	if perPage <= 0 || perPage > 100 {
		perPage = 20
	}
	page := input.Page
	if page <= 0 {
		page = 1
	}
	return s.messages.List(ctx, scope, repository.MessageListFilter{
		UserID:            userID,
		MailboxID:         input.MailboxID,
		Participant:       input.Participant,
		Direction:         input.Direction,
		RelatedEntityType: input.RelatedEntityType,
		RelatedEntityID:   input.RelatedEntityID,
		Limit:             perPage,
		Offset:            (page - 1) * perPage,
	})
}

func (s *MessageService) Get(ctx context.Context, scope coretenant.Scope, userID, id string) (domain.Message, error) {
	message, err := s.messages.GetForUser(ctx, scope, userID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Message{}, mailboxmodule.ErrMessageNotFound
	}
	return message, err
}

func (s *MessageService) AttachmentURL(ctx context.Context, scope coretenant.Scope, userID, messageID, attachmentID string) (assetservice.DownloadResult, error) {
	message, err := s.Get(ctx, scope, userID, messageID)
	if err != nil {
		return assetservice.DownloadResult{}, err
	}
	for _, attachment := range message.Attachments {
		if attachment.ID == attachmentID {
			return s.storage.DownloadURL(ctx, scope, attachment.AssetObjectID)
		}
	}
	return assetservice.DownloadResult{}, mailboxmodule.ErrAttachmentMissing
}

func readFiles(inputs []FileInput) ([]preparedFile, error) {
	var total int64
	files := make([]preparedFile, 0, len(inputs))
	for _, input := range inputs {
		total += input.Size
		if total > MaxTotalAttachmentBytes {
			return nil, mailboxmodule.ErrAttachmentsTooLarge
		}
		mimeType, ok := attachmentMimeType(input.Filename)
		if !ok {
			return nil, mailboxmodule.ErrAttachmentType
		}
		content, err := io.ReadAll(io.LimitReader(input.Content, MaxTotalAttachmentBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(content)) > MaxTotalAttachmentBytes {
			return nil, mailboxmodule.ErrAttachmentsTooLarge
		}
		files = append(files, preparedFile{filename: input.Filename, mimeType: mimeType, content: content})
	}
	return files, nil
}

func validateRecipients(to, cc, bcc []string) ([]netmail.Address, []netmail.Address, []netmail.Address, error) {
	parse := func(field string, values []string) ([]netmail.Address, error) {
		out := []netmail.Address{}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			address, err := netmail.ParseAddress(value)
			if err != nil {
				return nil, validationError(fmt.Sprintf("%s contains an invalid email address: %s", field, value))
			}
			address.Address = strings.ToLower(address.Address)
			out = append(out, *address)
		}
		return out, nil
	}
	toList, err := parse("to", to)
	if err != nil {
		return nil, nil, nil, err
	}
	ccList, err := parse("cc", cc)
	if err != nil {
		return nil, nil, nil, err
	}
	bccList, err := parse("bcc", bcc)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(toList) == 0 {
		return nil, nil, nil, validationError("at least one recipient is required")
	}
	if len(toList)+len(ccList)+len(bccList) > maxRecipients {
		return nil, nil, nil, validationError(fmt.Sprintf("at most %d recipients", maxRecipients))
	}
	return toList, ccList, bccList, nil
}

func addressStrings(addresses []netmail.Address) []string {
	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		out = append(out, address.Address)
	}
	return out
}

func mimeAttachments(files []preparedFile) []platformmail.Attachment {
	out := make([]platformmail.Attachment, 0, len(files))
	for _, file := range files {
		out = append(out, platformmail.Attachment{Filename: file.filename, ContentType: file.mimeType, Content: file.content})
	}
	return out
}

func domainOf(email string) string {
	if at := strings.LastIndex(email, "@"); at >= 0 {
		return email[at+1:]
	}
	return ""
}

func nonEmpty(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

func truncate(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max-1]) + "…"
}
