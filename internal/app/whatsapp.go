package app

import (
	"log/slog"
	"strings"
	"time"

	"zyad.cloud/internal/config"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	notificationrepo "zyad.cloud/internal/core/notification/repository"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	whatsapphandler "zyad.cloud/internal/modules/whatsapp/handler"
	whatsapprepo "zyad.cloud/internal/modules/whatsapp/repository"
	whatsappservice "zyad.cloud/internal/modules/whatsapp/service"
	"zyad.cloud/internal/platform/database"
	redisplatform "zyad.cloud/internal/platform/redis"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
)

// NewWAHAProvider returns the WAHA client when WHATSAPP_PROVIDER=waha and
// credentials are set, otherwise nil (the session service then answers
// provider actions with WHATSAPP_NOT_CONFIGURED). Shared by cmd/api and
// cmd/worker.
func NewWAHAProvider(cfg config.WhatsAppConfig) *platformwhatsapp.WAHAClient {
	wahaConfig := platformwhatsapp.WAHAConfig{
		BaseURL:        cfg.URL,
		APIKey:         cfg.APIKey,
		DefaultSession: cfg.Session,
		Timeout:        time.Duration(cfg.TimeoutSeconds) * time.Second,
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.Provider), platformwhatsapp.ProviderWAHA) || !wahaConfig.HasCredentials() {
		return nil
	}
	return platformwhatsapp.NewWAHAClient(wahaConfig)
}

// NewWhatsAppSessionService wires the session service, including the email
// notification to organization owners when a session disconnects. quota may
// be nil (worker: it never creates sessions).
func NewWhatsAppSessionService(
	cfg config.Config,
	db *database.Pool,
	quota whatsappservice.SessionQuotaGuard,
	log *slog.Logger,
) *whatsappservice.SessionService {
	var provider whatsappservice.SessionProvider
	if client := NewWAHAProvider(cfg.WhatsApp); client != nil {
		provider = client
	}
	notifier := whatsappservice.NewOwnerDisconnectNotifier(
		whatsapprepo.NewOwnerRepository(db),
		notificationpublisher.NewOutboxPublisher(notificationrepo.NewOutboxRepository(db), cfg.Notification.MaxAttempts),
		whatsappservice.DisconnectNotifierConfig{
			AppName:     cfg.App.Name,
			FrontendURL: cfg.App.FrontendURL,
			Locale:      cfg.Notification.DefaultLocale,
			MaxAttempts: cfg.Notification.MaxAttempts,
		},
	)
	options := []whatsappservice.SessionServiceOption{
		whatsappservice.WithSessionLogger(log),
		whatsappservice.WithDisconnectNotifier(notifier),
	}
	if quota != nil {
		options = append(options, whatsappservice.WithSessionQuotaGuard(quota))
	}
	return whatsappservice.NewSessionService(
		whatsapprepo.NewSessionRepository(db),
		provider,
		whatsappservice.SessionServiceConfig{
			WebhookURL:     cfg.WhatsApp.CallbackURL,
			WebhookHMACKey: cfg.WhatsApp.WebhookHMACKey,
			Engine:         cfg.WhatsApp.Engine,
		},
		options...,
	)
}

// NewWhatsAppWebhookHandler builds the public WAHA webhook receiver.
func NewWhatsAppWebhookHandler(cfg config.Config, db *database.Pool, log *slog.Logger) *whatsapphandler.WebhookHandler {
	return whatsapphandler.NewWebhookHandler(
		whatsapprepo.NewDirectoryRepository(db),
		whatsapprepo.NewWebhookEventRepository(db),
		cfg.WhatsApp.WebhookHMACKey,
		log,
	)
}

// NewWhatsAppWorkerResolver resolves organizations for the whatsapp worker
// identity.
func NewWhatsAppWorkerResolver(db *database.Pool) *organizationservice.WorkerResolver {
	return organizationservice.NewWorkerResolver(
		organizationrepo.NewOrganizationRepository(db),
		whatsappservice.WorkerIdentity,
	)
}

// NewWhatsAppInboundProcessor builds the worker-side webhook processor, or
// nil when WAHA is not configured.
func NewWhatsAppInboundProcessor(cfg config.Config, db *database.Pool, log *slog.Logger) *whatsappservice.InboundProcessor {
	client := NewWAHAProvider(cfg.WhatsApp)
	if client == nil {
		return nil
	}
	return whatsappservice.NewInboundProcessor(whatsappservice.InboundProcessorDeps{
		Events:        whatsapprepo.NewWebhookEventRepository(db),
		Directory:     whatsapprepo.NewDirectoryRepository(db),
		Resolver:      NewWhatsAppWorkerResolver(db),
		Sessions:      whatsapprepo.NewSessionRepository(db),
		SessionSvc:    NewWhatsAppSessionService(cfg, db, nil, log),
		Conversations: whatsapprepo.NewConversationRepository(db),
		LIDs:          client,
		CRM:           NewWhatsAppCRMGateway(db),
		Logger:        log,
	})
}

// NewWhatsAppCRMGateway gives the whatsapp module its CRM access.
func NewWhatsAppCRMGateway(db *database.Pool) *whatsappservice.CRMGateway {
	return whatsappservice.NewCRMGateway(
		crmrepo.NewLeadRepository(db),
		crmrepo.NewContactRepository(db),
		crmrepo.NewActivityRepository(db),
		crmrepo.NewMemberRepository(db),
	)
}

// NewWhatsAppConversationService wires conversations and sending. Sending
// is rate limited per session in Redis (WHATSAPP_SEND_RATE_PER_MINUTE).
func NewWhatsAppConversationService(cfg config.Config, db *database.Pool, redis *redisplatform.Client, log *slog.Logger) *whatsappservice.ConversationService {
	deps := whatsappservice.ConversationServiceDeps{
		Conversations: whatsapprepo.NewConversationRepository(db),
		Sessions:      whatsapprepo.NewSessionRepository(db),
		CRM:           NewWhatsAppCRMGateway(db),
		Logger:        log,
	}
	if client := NewWAHAProvider(cfg.WhatsApp); client != nil {
		deps.Sender = client
	}
	if redis != nil {
		deps.Limiter = whatsappservice.NewRedisSendRateLimiter(redis, cfg.WhatsApp.SendRatePerMinute, log)
	}
	return whatsappservice.NewConversationService(deps)
}

// NewWhatsAppNotificationClient is the client behind the notification
// WhatsAppDispatcher: WAHA via the platform organization's "notification"
// session when WAHA is configured, otherwise the NoopClient.
func NewWhatsAppNotificationClient(cfg config.Config, db *database.Pool, log *slog.Logger) platformwhatsapp.Client {
	client := NewWAHAProvider(cfg.WhatsApp)
	if client == nil {
		return platformwhatsapp.NewNoopClient()
	}
	return whatsappservice.NewNotificationClient(
		client,
		whatsapprepo.NewSessionRepository(db),
		NewWhatsAppWorkerResolver(db),
		cfg.MultiTenant.PlatformOrganizationID,
		cfg.WhatsApp.Session,
		log,
	)
}
