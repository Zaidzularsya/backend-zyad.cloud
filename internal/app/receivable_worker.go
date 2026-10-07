package app

import (
	"fmt"
	"log/slog"

	"zyad.cloud/internal/config"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	notificationrepo "zyad.cloud/internal/core/notification/repository"
	permissionrepo "zyad.cloud/internal/core/permission/repository"
	permissionservice "zyad.cloud/internal/core/permission/service"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	crmservice "zyad.cloud/internal/modules/crm/service"
	mailboxrepo "zyad.cloud/internal/modules/mailbox/repository"
	mailboxservice "zyad.cloud/internal/modules/mailbox/service"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	receivablerepo "zyad.cloud/internal/modules/receivable/repository"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	subscriptionrepo "zyad.cloud/internal/modules/subscription/repository"
	subscriptionservice "zyad.cloud/internal/modules/subscription/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/shared/publiclink"
)

// NewReceivableBillingRunner merakit job billing run per jam (cmd/worker saja). Service invoice dirakit
// sama seperti di API karena penerbitan memakai PDF, link publik, dan kirim otomatis (email/WhatsApp);
// bedanya worker tidak punya Redis, sehingga WhatsApp berjalan tanpa rate limiter dan tanpa realtime bus.
// Mengembalikan galat bila storage tenant belum dikonfigurasi (PDF snapshot butuh asset store).
func NewReceivableBillingRunner(cfg config.Config, db *database.Pool, log *slog.Logger) (*receivableservice.BillingRunner, error) {
	assetSvc, err := NewAssetService(cfg, db, log)
	if err != nil {
		return nil, fmt.Errorf("receivable billing run: asset storage: %w", err)
	}

	leadRepo, contactRepo, activityRepo := crmrepo.NewLeadRepository(db), crmrepo.NewContactRepository(db), crmrepo.NewActivityRepository(db)
	mailboxRepo, messageRepo := mailboxrepo.NewMailboxRepository(db), mailboxrepo.NewMessageRepository(db)
	mailboxSvc := mailboxservice.NewMailboxService(mailboxRepo, messageRepo, assetSvc, mailboxservice.MailboxServiceOptions{
		SecretKey: cfg.App.Secret, AllowPrivateHosts: cfg.Mail.MailboxAllowPrivateHosts,
	})
	messageSvc := mailboxservice.NewMessageService(mailboxRepo, messageRepo, assetSvc,
		mailboxservice.NewCRMGateway(leadRepo, contactRepo, activityRepo),
		mailboxservice.MessageServiceOptions{SecretKey: cfg.App.Secret, AllowPrivateHosts: cfg.Mail.MailboxAllowPrivateHosts, Log: log})

	entitlements := organizationservice.NewEntitlementService(organizationrepo.NewEntitlementRepository(db))
	guard := subscriptionservice.NewSubscriptionGuardService(
		subscriptionrepo.NewSubscriptionRepository(db), entitlements,
		subscriptionservice.WithOrganizationTypeResolver(organizationrepo.NewOrganizationRepository(db)),
	)

	module := buildReceivable(receivableBuild{
		DB: db, Assets: assetSvc, Links: publiclink.NewService(publiclink.NewRepository(db), cfg.App.Secret),
		Mailboxes: mailboxSvc, Messages: messageSvc,
		WhatsApp:     NewWhatsAppConversationService(cfg, db, nil, nil, log),
		Entitlements: subscriptionEntitlementChecker{guard: guard},
		Permissions:  permissionservice.New(permissionrepo.New(db)),
		Publisher:    notificationpublisher.NewOutboxPublisher(notificationrepo.NewOutboxRepository(db), cfg.Notification.MaxAttempts),
		Members:      crmrepo.NewMemberRepository(db), Issuers: crmrepo.NewQuotationIssuerRepository(db),
		AppName: cfg.App.Name, FrontendURL: cfg.App.FrontendURL, NotificationLocale: cfg.Notification.DefaultLocale,
	})

	// Billing run mengakhiri contract; listener CRM mencabut akses workspace-nya.
	module.Listeners.Add(crmservice.NewReceivableListener(crmrepo.NewSalesOrderRepository(db), nil, nil).
		WithTenantAccess(newCRMTenantAccess(db, module.Invoices, module.Contracts, nil)))
	run := receivableservice.NewBillingRun(receivablerepo.NewContractRepository(db), module.Invoices, receivablerepo.NewSettingsRepository(db), module.Listeners, nil)
	scopes := organizationservice.NewWorkerResolver(organizationrepo.NewOrganizationRepository(db), receivableservice.BillingRunWorkerIdentity)
	return receivableservice.NewBillingRunner(receivableOrgLister{repo: organizationrepo.NewOrganizationRepository(db)}, scopes, run), nil
}
