package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	corecrypto "zyad.cloud/internal/core/crypto"
	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	mailboxmodule "zyad.cloud/internal/modules/mailbox"
	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/modules/mailbox/repository"
)

// SyncWorkerIdentity is the internal service identity the mail sync worker
// uses with organization/service.WorkerResolver.
const SyncWorkerIdentity = "mailbox-sync-worker"

// syncFolder is the only IMAP folder synced (Fase 3 scope): providers name
// Sent differently (Gmail: "[Gmail]/Sent Mail"), and sent mail is already
// tracked as outbound messages this app creates when sending.
const syncFolder = "INBOX"

type SyncWorkerTenantResolver interface {
	ResolveWorkerOrganization(ctx context.Context, organizationID string, serviceIdentity string) (coretenant.Context, error)
}

type SyncResult struct {
	Checked int
	Synced  int
	Failed  int
}

type SyncServiceOptions struct {
	SecretKey         string
	AllowPrivateHosts bool
	Dial              IMAPDialer
	InitialSyncDays   int
	BatchLimit        int
	Log               *slog.Logger
	Now               func() time.Time
}

// SyncService pulls new INBOX messages for every connected mailbox over
// IMAP. It runs from the worker only (see cmd/worker), one mailbox at a
// time; a failure on one mailbox does not stop the others.
type SyncService struct {
	directory       repository.DirectoryRepository
	mailboxes       repository.MailboxRepository
	messages        repository.MessageRepository
	storage         AttachmentStorage
	resolver        SyncWorkerTenantResolver
	secretKey       string
	allowPrivate    bool
	dial            IMAPDialer
	initialSyncDays int
	batchLimit      int
	log             *slog.Logger
	now             func() time.Time
}

func NewSyncService(
	directory repository.DirectoryRepository,
	mailboxes repository.MailboxRepository,
	messages repository.MessageRepository,
	storage AttachmentStorage,
	resolver SyncWorkerTenantResolver,
	options SyncServiceOptions,
) *SyncService {
	dial := options.Dial
	if dial == nil {
		dial = dialIMAP
	}
	log := options.Log
	if log == nil {
		log = slog.Default()
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	initialSyncDays := options.InitialSyncDays
	if initialSyncDays <= 0 {
		initialSyncDays = 30
	}
	batchLimit := options.BatchLimit
	if batchLimit <= 0 {
		batchLimit = 200
	}
	return &SyncService{
		directory: directory, mailboxes: mailboxes, messages: messages, storage: storage, resolver: resolver,
		secretKey: options.SecretKey, allowPrivate: options.AllowPrivateHosts, dial: dial,
		initialSyncDays: initialSyncDays, batchLimit: batchLimit, log: log, now: now,
	}
}

// RunOnce syncs every mailbox that has IMAP settings once.
func (s *SyncService) RunOnce(ctx context.Context) (SyncResult, error) {
	entries, err := s.directory.ListAll(ctx)
	if err != nil {
		return SyncResult{}, err
	}

	result := SyncResult{}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		synced, err := s.syncOne(ctx, entry)
		if err != nil {
			if errors.Is(err, errSkipMailbox) {
				continue
			}
			result.Checked++
			result.Failed++
			s.log.Warn("mailbox sync failed", "mailbox_id", entry.MailboxID, "organization_id", entry.OrganizationID, "error", err)
			continue
		}
		result.Checked++
		if synced {
			result.Synced++
		}
	}
	return result, nil
}

// errSkipMailbox marks a mailbox this pass has nothing to do for (disabled,
// no IMAP configured) — not a failure, so it's not logged or counted.
var errSkipMailbox = errors.New("mailbox sync: skip")

func (s *SyncService) syncOne(ctx context.Context, entry repository.DirectoryEntry) (synced bool, err error) {
	tenantContext, err := s.resolver.ResolveWorkerOrganization(ctx, entry.OrganizationID, SyncWorkerIdentity)
	if err != nil {
		return false, err
	}
	scope, err := coretenant.NewScope(tenantContext)
	if err != nil {
		return false, err
	}
	ctx = coretenant.WithContext(ctx, tenantContext)

	mailbox, err := s.mailboxes.GetByID(ctx, scope, entry.MailboxID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, errSkipMailbox
		}
		return false, err
	}
	if mailbox.Status == domain.MailboxStatusDisabled || !mailbox.HasIMAP() {
		return false, errSkipMailbox
	}
	return s.syncMailbox(ctx, scope, mailbox)
}

// SyncMailbox runs one sync pass for a single mailbox the caller already
// owns (an authenticated "sync now" request — see mailbox/handler.go). It
// skips the worker-only organization resolution syncOne does, since the
// caller already has a verified tenant scope.
func (s *SyncService) SyncMailbox(ctx context.Context, scope coretenant.Scope, userID, mailboxID string) (bool, error) {
	mailbox, err := s.mailboxes.GetForUser(ctx, scope, userID, mailboxID)
	if err != nil {
		return false, err
	}
	if !mailbox.HasIMAP() {
		return false, mailboxmodule.ErrIMAPNotConfigured
	}
	synced, err := s.syncMailbox(ctx, scope, mailbox)
	if err != nil {
		// The raw error (mailbox.SetStatus already recorded it for the
		// owner to read) may name internal hosts; don't put it on the
		// response.
		return false, mailboxmodule.ErrSyncFailed
	}
	return synced, nil
}

func (s *SyncService) syncMailbox(ctx context.Context, scope coretenant.Scope, mailbox domain.Mailbox) (synced bool, err error) {
	password, err := corecrypto.DecryptSecret(s.secretKey, mailbox.SecretEncrypted)
	if err != nil {
		return false, err
	}

	conn, err := s.dial(ctx, IMAPDialConfig{
		Host: mailbox.IMAPHost, Port: mailbox.IMAPPort, Security: mailbox.IMAPSecurity,
		Username: mailbox.Username, Password: password,
	}, s.allowPrivate)
	if err != nil {
		s.markError(ctx, scope, mailbox.ID, err)
		return false, err
	}
	defer conn.Close()

	cursor := mailbox.SyncCursor[syncFolder]
	if cursor.UIDValidity != 0 && cursor.UIDValidity != conn.UIDValidity() {
		// The folder was recreated (e.g. account migration): old UIDs mean
		// nothing anymore, restart from the initial-sync window.
		s.log.Info("mailbox uidvalidity changed, resyncing", "mailbox_id", mailbox.ID)
		cursor = domain.FolderCursor{}
	}

	cutoff := s.now().AddDate(0, 0, -s.initialSyncDays)
	fetched, err := conn.FetchSince(ctx, cursor.LastUID, cutoff, s.batchLimit)
	if err != nil {
		s.markError(ctx, scope, mailbox.ID, err)
		return false, err
	}

	lastUID := cursor.LastUID
	for _, message := range fetched {
		if err := s.storeMessage(ctx, scope, mailbox.ID, message); err != nil {
			// Keep what synced so far; the failed message (and anything
			// after it) is retried on the next pass.
			s.persistCursor(ctx, scope, mailbox.ID, conn.UIDValidity(), lastUID)
			return lastUID > cursor.LastUID, err
		}
		lastUID = message.UID
	}
	s.persistCursor(ctx, scope, mailbox.ID, conn.UIDValidity(), lastUID)
	return len(fetched) > 0, nil
}

func (s *SyncService) storeMessage(ctx context.Context, scope coretenant.Scope, mailboxID string, fetched FetchedMessage) error {
	message, created, err := s.messages.CreateInbound(ctx, scope, repository.CreateInboundParams{
		MailboxID: mailboxID, MessageID: fetched.MessageID, InReplyTo: fetched.InReplyTo, References: fetched.InReplyTo,
		FromAddress: fetched.FromAddress, FromName: fetched.FromName, To: fetched.To, Cc: fetched.Cc, Bcc: fetched.Bcc,
		Subject: fetched.Subject, Snippet: snippet(htmlToText(sanitizeHTML(fetched.BodyHTML))+fetched.BodyText, 200),
		BodyHTML: sanitizeHTML(fetched.BodyHTML), BodyText: fetched.BodyText, SentAt: fetched.Date,
	})
	if err != nil {
		return err
	}
	if !created {
		return nil
	}
	for _, attachment := range fetched.Attachments {
		if err := s.storeAttachment(ctx, scope, message.ID, attachment); err != nil {
			// One unreadable/disallowed attachment doesn't lose the email.
			s.log.Warn("inbound attachment skipped", "message_id", message.ID, "filename", attachment.Filename, "error", err)
		}
	}
	return nil
}

func (s *SyncService) storeAttachment(ctx context.Context, scope coretenant.Scope, messageID string, attachment FetchedAttachment) error {
	object, err := s.storage.UploadObject(ctx, scope, assetservice.UploadObjectParams{
		Filename: attachment.Filename, MimeType: attachment.ContentType, SizeBytes: int64(len(attachment.Content)),
		Class: assetdomain.ObjectClassPrivate, Label: "mail_message:" + messageID, AllowedMimeTypes: attachmentMimeTypes,
	}, bytes.NewReader(attachment.Content))
	if err != nil {
		return err
	}
	if err := s.messages.AddAttachment(ctx, scope, messageID, object.ID); err != nil {
		_ = s.storage.DeleteObject(ctx, scope, object.ID)
		return err
	}
	return nil
}

func (s *SyncService) persistCursor(ctx context.Context, scope coretenant.Scope, mailboxID string, uidValidity, lastUID uint32) {
	cursor := domain.SyncCursor{syncFolder: domain.FolderCursor{UIDValidity: uidValidity, LastUID: lastUID}}
	if err := s.mailboxes.UpdateSyncCursor(ctx, scope, mailboxID, cursor, s.now().UTC()); err != nil {
		s.log.Error("persist mailbox sync cursor", "mailbox_id", mailboxID, "error", err)
	}
}

func (s *SyncService) markError(ctx context.Context, scope coretenant.Scope, mailboxID string, syncErr error) {
	if err := s.mailboxes.SetStatus(ctx, scope, mailboxID, domain.MailboxStatusError, syncErr.Error()); err != nil {
		s.log.Error("mark mailbox sync error", "mailbox_id", mailboxID, "error", err)
	}
}
