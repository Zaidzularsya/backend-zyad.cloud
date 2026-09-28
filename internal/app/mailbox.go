package app

import (
	"log/slog"

	"zyad.cloud/internal/config"
	assetservice "zyad.cloud/internal/modules/asset/service"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	mailboxrepo "zyad.cloud/internal/modules/mailbox/repository"
	mailboxservice "zyad.cloud/internal/modules/mailbox/service"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	"zyad.cloud/internal/platform/database"
)

// NewMailboxCRMGateway gives the mailbox module its CRM access (checking the
// lead/contact an email is sent to, logging sent/synced email on its
// timeline).
func NewMailboxCRMGateway(db *database.Pool) *mailboxservice.CRMGateway {
	return mailboxservice.NewCRMGateway(
		crmrepo.NewLeadRepository(db),
		crmrepo.NewContactRepository(db),
		crmrepo.NewActivityRepository(db),
	)
}

// NewMailSyncWorkerResolver resolves organizations for the mail sync worker
// identity (cmd/worker only).
func NewMailSyncWorkerResolver(db *database.Pool) *organizationservice.WorkerResolver {
	return organizationservice.NewWorkerResolver(
		organizationrepo.NewOrganizationRepository(db),
		mailboxservice.SyncWorkerIdentity,
	)
}

// NewMailSyncServiceWithStorage builds the IMAP inbox sync service given an
// already-constructed asset service (internal/app's own big constructor
// already builds one for everything else). Used by both cmd/api ("sync
// now", independent of the worker's interval) and NewMailSyncService.
func NewMailSyncServiceWithStorage(cfg config.Config, db *database.Pool, assetSvc assetservice.AssetService, log *slog.Logger) *mailboxservice.SyncService {
	return mailboxservice.NewSyncService(
		mailboxrepo.NewDirectoryRepository(db),
		mailboxrepo.NewMailboxRepository(db),
		mailboxrepo.NewMessageRepository(db),
		assetSvc,
		NewMailSyncWorkerResolver(db),
		mailboxservice.SyncServiceOptions{
			SecretKey:         cfg.App.Secret,
			AllowPrivateHosts: cfg.Mail.MailboxAllowPrivateHosts,
			InitialSyncDays:   cfg.Mail.MailSyncInitialDays,
			BatchLimit:        cfg.Mail.MailSyncBatchLimit,
			Log:               log,
		},
	)
}

// NewMailSyncService builds its own asset service too (cmd/worker only,
// which has no other reason to build one) — nil when tenant storage isn't
// configured (mirrors NewWAHAProvider returning nil for WhatsApp), so the
// worker simply doesn't start the sync loop.
func NewMailSyncService(cfg config.Config, db *database.Pool, log *slog.Logger) *mailboxservice.SyncService {
	assetSvc, err := NewAssetService(cfg, db, log)
	if err != nil {
		log.Warn("mail sync disabled: asset storage not configured", "error", err)
		return nil
	}
	return NewMailSyncServiceWithStorage(cfg, db, assetSvc, log)
}
