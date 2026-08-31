package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"zyad.cloud/internal/config"
	coreauth "zyad.cloud/internal/core/auth"
	notificationdispatcher "zyad.cloud/internal/core/notification/dispatcher"
	"zyad.cloud/internal/core/notification/domain"
	notificationhandler "zyad.cloud/internal/core/notification/handler"
	notificationpublisher "zyad.cloud/internal/core/notification/publisher"
	notificationrepo "zyad.cloud/internal/core/notification/repository"
	notificationservice "zyad.cloud/internal/core/notification/service"
	notificationtemplate "zyad.cloud/internal/core/notification/template"
	permissionhandler "zyad.cloud/internal/core/permission/handler"
	permissionrepo "zyad.cloud/internal/core/permission/repository"
	permissionservice "zyad.cloud/internal/core/permission/service"
	corevalidation "zyad.cloud/internal/core/validation"
	billinghandler "zyad.cloud/internal/modules/billing/handler"
	billingrepo "zyad.cloud/internal/modules/billing/repository"
	billingservice "zyad.cloud/internal/modules/billing/service"
	landinghandler "zyad.cloud/internal/modules/landing/handler"
	landingrepo "zyad.cloud/internal/modules/landing/repository"
	landingservice "zyad.cloud/internal/modules/landing/service"
	organizationhandler "zyad.cloud/internal/modules/organization/handler"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	producthandler "zyad.cloud/internal/modules/product/handler"
	productrepo "zyad.cloud/internal/modules/product/repository"
	productservice "zyad.cloud/internal/modules/product/service"
	subscriptionhandler "zyad.cloud/internal/modules/subscription/handler"
	subscriptionrepo "zyad.cloud/internal/modules/subscription/repository"
	subscriptionservice "zyad.cloud/internal/modules/subscription/service"
	userhandler "zyad.cloud/internal/modules/user/handler"
	userrepo "zyad.cloud/internal/modules/user/repository"
	userservice "zyad.cloud/internal/modules/user/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/doku"
	"zyad.cloud/internal/platform/logger"
	"zyad.cloud/internal/platform/mail"
	redisplatform "zyad.cloud/internal/platform/redis"
	"zyad.cloud/internal/platform/storage"
	"zyad.cloud/internal/platform/whatsapp"
)

type App struct {
	config config.Config
	logger *slog.Logger
	db     *database.Pool
	redis  *redisplatform.Client
	router http.Handler
	server *http.Server
}

func New(ctx context.Context) (*App, error) {
	cfg := config.Load()
	if err := cfg.ValidateForApp(); err != nil {
		return nil, err
	}

	log := logger.New(cfg.App.Env)
	corevalidation.RegisterGinValidators()

	var db *database.Pool
	var redisClient *redisplatform.Client
	var err error

	if db, err = database.Connect(ctx, cfg.Database, log); err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	if redisClient, err = redisplatform.Connect(ctx, cfg.Redis, log); err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	permRepo := permissionrepo.New(db)
	permService := permissionservice.New(permRepo)
	permHandler := permissionhandler.New(permService)

	productPlanRepo := productrepo.NewPlanRepository(db)
	productFeatureRepo := productrepo.NewFeatureRepository(db)
	productPlanEntitlementRepo := productrepo.NewPlanEntitlementRepository(db)
	subscriptionRepo := subscriptionrepo.NewSubscriptionRepository(db)
	billingInvoiceRepo := billingrepo.NewInvoiceRepository(db)
	billingPaymentRepo := billingrepo.NewPaymentRepository(db)
	subscriptionEntitlementSink := subscriptionrepo.NewEntitlementSink(db)

	productPlanService := productservice.NewPlanService(productPlanRepo, productPlanEntitlementRepo)
	productFeatureService := productservice.NewFeatureService(productFeatureRepo)
	productPlanEntitlementService := productservice.NewPlanEntitlementService(
		productPlanEntitlementRepo,
		productFeatureRepo,
	)
	subscriptionService := subscriptionservice.NewSubscriptionService(
		subscriptionRepo,
		productPlanEntitlementRepo,
		subscriptionEntitlementSink,
	)
	billingInvoiceService := billingservice.NewInvoiceService(billingInvoiceRepo)
	billingPaymentService := billingservice.NewPaymentService(
		billingPaymentRepo,
		billingInvoiceRepo,
		subscriptionUpgradeActivatorAdapter{subscriptions: subscriptionService},
	)
	dokuClient := doku.NewClientFromConfig(doku.Config{
		BaseURL:     cfg.Doku.BaseURL,
		ClientID:    cfg.Doku.ClientID,
		SecretKey:   cfg.Doku.SecretKey,
		FrontendURL: cfg.App.FrontendURL,
	})
	dokuNotificationURL := ""
	if baseURL := strings.TrimRight(strings.TrimSpace(cfg.App.URL), "/"); baseURL != "" {
		dokuNotificationURL = baseURL + "/api/v1/webhooks/doku"
	}
	billingPaymentService.SetDokuCheckout(dokuClient, cfg.App.FrontendURL, dokuNotificationURL)
	dokuWebhookHandler := billinghandler.NewDokuWebhookHandler(
		billingPaymentService,
		cfg.Doku.ClientID,
		cfg.Doku.SecretKey,
		log,
	)
	platformProductHandler := producthandler.NewPlatformProductHandler(
		productPlanService,
		productFeatureService,
		productPlanEntitlementService,
		permService,
	)
	publicProductHandler := producthandler.NewPublicProductHandler(productPlanService)
	platformSubscriptionHandler := subscriptionhandler.NewPlatformSubscriptionHandler(
		subscriptionService,
		permService,
	)
	platformBillingHandler := billinghandler.NewPlatformBillingHandler(
		billingInvoiceService,
		billingPaymentService,
		permService,
	)

	templateRepo := notificationrepo.NewTemplateRepository(db)
	templateRegistry := notificationtemplate.NewVariableRegistry()
	templateValidator := notificationtemplate.NewValidator(templateRegistry)
	templateRenderer := notificationtemplate.NewRenderer(templateRegistry)
	templateService := notificationservice.NewTemplateService(templateRepo, templateValidator, templateRenderer)
	templateHandler := notificationhandler.NewTemplateHandler(templateService, permService)
	variableHandler := notificationhandler.NewVariableHandler(templateRegistry, permService)
	logRepo := notificationrepo.NewNotificationLogRepository(db)
	preferenceRepo := notificationrepo.NewPreferenceRepository(db)
	outboxRepo := notificationrepo.NewOutboxRepository(db)
	preferenceService := notificationservice.NewPreferenceService(preferenceRepo)
	notificationService := notificationservice.NewNotificationService(
		templateRepo,
		logRepo,
		preferenceRepo,
		templateRenderer,
		notificationdispatcher.NewEmailDispatcher(mail.NewMailerFromConfig(cfg.Mail)),
		notificationdispatcher.NewWhatsAppDispatcher(whatsapp.NewNoopClient()),
		notificationdispatcher.NewNoopDispatcher(domain.ChannelInApp),
		notificationdispatcher.NewNoopDispatcher(domain.ChannelDiscord),
	)
	notificationHandler := notificationhandler.NewNotificationHandler(notificationService)
	logHandler := notificationhandler.NewLogHandler(notificationService, permService)
	preferenceHandler := notificationhandler.NewPreferenceHandler(preferenceService, permService)

	authRepo := userrepo.NewAuthRepository(db)
	authService, err := userservice.NewAuthService(authRepo, cfg)
	if err != nil {
		return nil, fmt.Errorf("create auth service: %w", err)
	}
	if cfg.Auth.Google.Enabled {
		authService.SetGoogleTokenVerifier(coreauth.NewGoogleIDTokenVerifier(cfg.Auth.Google.ClientIDs))
	}
	authService.SetNotificationPublisher(notificationpublisher.NewOutboxPublisher(outboxRepo, cfg.Notification.MaxAttempts))
	authHandler := userhandler.NewAuthHandler(authService, redisClient, cfg.App.FrontendURL)
	userRepo := userrepo.NewUserRepository(db)
	userService := userservice.NewUserService(userRepo)
	if err := userService.Configure(cfg); err != nil {
		return nil, fmt.Errorf("configure user service: %w", err)
	}
	userService.SetNotificationPublisher(notificationpublisher.NewOutboxPublisher(outboxRepo, cfg.Notification.MaxAttempts))
	userHandler := userhandler.NewUserHandler(userService, authService, permService)
	organizationResolverRepo := organizationrepo.NewAuthenticatedResolverRepository(db)
	organizationResolver := organizationservice.NewAuthenticatedResolver(organizationResolverRepo)
	publicHostResolverRepo := organizationrepo.NewPublicHostResolverRepository(db)
	publicHostResolver := organizationservice.NewPublicHostResolver(
		publicHostResolverRepo,
		cfg.MultiTenant.PlatformOrganizationID,
		cfg.MultiTenant.PlatformPrimaryDomain,
	)
	organizationSwitchService := organizationservice.NewSwitchService(
		organizationrepo.NewSwitchRepository(db),
	)
	organizationSwitchHandler := organizationhandler.NewSwitchHandler(
		organizationSwitchService,
	)
	organizationOnboardingService := organizationservice.NewOnboardingService(
		organizationrepo.NewOnboardingRepository(db),
	)
	defaultSubscriptions := defaultSubscriptionProvisioner{
		plans:         productPlanService,
		subscriptions: subscriptionService,
	}
	organizationOnboardingService.SetDefaultSubscriptionProvisioner(defaultSubscriptions)
	authService.SetGoogleWorkspaceProvisioner(
		googleWorkspaceProvisioner{onboarding: organizationOnboardingService},
	)
	organizationOnboardingHandler := organizationhandler.NewOnboardingHandler(
		organizationOnboardingService,
	)
	organizationLifecycleService := organizationservice.NewLifecycleService(
		organizationrepo.NewLifecycleRepository(db),
	)
	organizationLifecycleService.SetNotificationPublisher(
		notificationpublisher.NewOutboxPublisher(outboxRepo, cfg.Notification.MaxAttempts),
	)
	organizationPlatformService := organizationservice.NewPlatformService(
		organizationrepo.NewOrganizationRepository(db),
		organizationLifecycleService,
	)
	organizationPlatformHandler := organizationhandler.NewPlatformHandler(
		organizationPlatformService,
		permService,
	)
	organizationMembershipRepository := organizationrepo.NewMembershipServiceRepository(db)
	organizationEntitlementRepository := organizationrepo.NewEntitlementRepository(db)
	organizationEntitlementRuntimeService := organizationservice.NewEntitlementService(
		organizationEntitlementRepository,
	)
	organizationEntitlementService := organizationservice.NewEntitlementAPIService(
		organizationEntitlementRepository,
		organizationEntitlementRuntimeService,
	)
	organizationEntitlementHandler := organizationhandler.NewEntitlementHandler(
		organizationEntitlementService,
		permService,
		permService,
	)
	subscriptionGuardService := subscriptionservice.NewSubscriptionGuardService(
		subscriptionRepo,
		organizationEntitlementRuntimeService,
	)
	organizationSelfService := organizationservice.NewSelfService(
		organizationrepo.NewSelfRepository(db),
		organizationservice.NewMembershipService(
			organizationMembershipRepository,
			organizationservice.WithMembershipBillingGuard(subscriptionGuardService),
		),
		organizationMembershipRepository,
	)
	organizationSelfHandler := organizationhandler.NewSelfHandler(
		organizationSelfService,
		permService,
	)
	organizationDomainService := organizationservice.NewDomainService(
		organizationrepo.NewDomainRepository(db),
		organizationservice.NewDNSDomainVerifier(nil),
		cfg.MultiTenant.PlatformPrimaryDomain,
		organizationservice.WithDomainBillingGuard(subscriptionGuardService),
	)
	organizationDomainHandler := organizationhandler.NewDomainHandler(
		organizationDomainService,
		permService,
	)
	tenantBillingService := billingservice.NewTenantBillingService(
		subscriptionService,
		productPlanService,
		billingInvoiceService,
		organizationEntitlementService,
	)
	tenantBillingService.SetDefaultSubscriptionProvisioner(defaultSubscriptions)
	tenantBillingHandler := billinghandler.NewTenantBillingHandler(
		tenantBillingService,
		permService,
	)
	tenantBillingHandler.SetCheckoutService(billingPaymentService)
	organizationImpersonationHandler := organizationhandler.NewImpersonationHandler(
		organizationservice.NewImpersonationService(
			organizationrepo.NewImpersonationRepository(db),
		),
		permService,
	)

	landingPageRepo := landingrepo.NewPageRepository(db)
	landingSectionRepo := landingrepo.NewSectionRepository(db)
	landingRevisionRepo := landingrepo.NewRevisionRepository(db)
	landingFormRepo := landingrepo.NewFormRepository(db)
	landingBrandingRepo := landingrepo.NewBrandingRepository(db)
	landingVersionRepo := landingrepo.NewVersionRepository(db)
	landingPlatformRepo := landingrepo.NewPlatformPageRepository(db)

	landingVisibilitySvc := landingservice.NewVisibilityService(landingPageRepo, landingPlatformRepo, redisClient, cfg)
	landingRevisionSvc := landingservice.NewRevisionService(landingRevisionRepo, landingPageRepo)
	landingPublishSvc := landingservice.NewPublishService(db, landingPageRepo, landingSectionRepo, landingFormRepo, landingBrandingRepo, landingVersionRepo, cfg.App.Secret)
	landingPageSvc := landingservice.NewPageService(
		landingPageRepo,
		landingSectionRepo,
		landingservice.WithLandingPageQuotaGuard(subscriptionGuardService),
		landingservice.WithLandingPageBrandingRepo(landingBrandingRepo),
	)
	landingSectionSvc := landingservice.NewSectionService(
		landingSectionRepo,
		landingservice.WithLandingSectionQuotaGuard(subscriptionGuardService),
	)

	landingDomainRepo := landingrepo.NewDomainRepository(db)
	landingDomainSvc := landingservice.NewDomainService(
		landingDomainRepo,
		landingPageRepo,
		db,
		landingservice.WithLandingDomainFeatureGate(subscriptionGuardService),
	)
	landingBrandingSvc := landingservice.NewBrandingService(landingBrandingRepo)
	landingFormSvc := landingservice.NewFormService(landingFormRepo)
	landingSubmissionRepo := landingrepo.NewSubmissionRepository(db)
	landingSubmissionSvc := landingservice.NewSubmissionService(landingSubmissionRepo, landingFormRepo)

	landingReusableRepo := landingrepo.NewReusableRepository(db)
	landingMediaRepo := landingrepo.NewMediaRepository(db)
	landingIntegrationRepo := landingrepo.NewIntegrationRepository(db)

	landingCTASvc := landingservice.NewCTAService(landingReusableRepo)
	landingTemplateSvc := landingservice.NewTemplateService(
		landingReusableRepo,
		landingSectionRepo,
		landingservice.WithTemplateSectionQuotaGuard(subscriptionGuardService),
	)
	mediaStorage, err := storage.NewLocalProvider(cfg.Storage.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("init media storage: %w", err)
	}
	landingMediaSvc := landingservice.NewMediaService(
		landingMediaRepo,
		landingservice.WithMediaObjectStorage(mediaStorage),
		landingservice.WithMediaPublicBaseURL(cfg.App.URL),
	)
	landingNavigationSvc := landingservice.NewNavigationService(landingReusableRepo, landingPageRepo)
	landingDeliverySvc := landingservice.NewDeliveryService(landingIntegrationRepo, landingSubmissionRepo)

	landingAdminPageHandler := landinghandler.NewAdminPageHandler(landingPageSvc, landingVisibilitySvc, landingRevisionSvc, landingPublishSvc)
	landingAdminSectionHandler := landinghandler.NewAdminSectionHandler(landingSectionSvc)
	landingAdminBrandingHandler := landinghandler.NewAdminBrandingHandler(landingBrandingSvc)
	landingAdminDomainHandler := landinghandler.NewAdminDomainHandler(landingDomainSvc)
	landingAdminFormHandler := landinghandler.NewAdminFormHandler(landingFormSvc)
	landingAdminSubmissionHandler := landinghandler.NewAdminSubmissionHandler(landingSubmissionSvc)
	landingAdminCTAHandler := landinghandler.NewAdminCTAHandler(landingCTASvc)
	landingAdminTemplateHandler := landinghandler.NewAdminTemplateHandler(landingTemplateSvc)
	landingAdminMediaHandler := landinghandler.NewAdminMediaHandler(landingMediaSvc)
	landingAdminNavigationHandler := landinghandler.NewAdminNavigationHandler(landingNavigationSvc)
	landingAdminRevisionHandler := landinghandler.NewAdminRevisionHandler(landingRevisionSvc)
	landingAdminScheduleHandler := landinghandler.NewAdminScheduleHandler(landingRevisionSvc)
	landingAdminIntegrationHandler := landinghandler.NewAdminIntegrationHandler(landingDeliverySvc)

	landingResolverRepo := landingrepo.NewResolverRepository(db)
	landingResolverSvc := landingservice.NewResolverService(db, landingResolverRepo, landingVersionRepo, landingPageRepo, landingSectionRepo, landingFormRepo, landingBrandingRepo, landingPublishSvc)
	landingAnalyticsRepo := landingrepo.NewAnalyticsRepository(db)
	landingAnalyticsSvc := landingservice.NewAnalyticsService(landingAnalyticsRepo, landingPageRepo, db)
	publicLandingHandler := landinghandler.NewPublicLandingHandler(landingResolverSvc, landingVisibilitySvc, landingSubmissionSvc, landingAnalyticsSvc)

	router, err := newRouter(Dependencies{
		Config:                           cfg,
		Logger:                           log,
		DB:                               db,
		Redis:                            redisClient,
		NotificationHandler:              notificationHandler,
		NotificationLogHandler:           logHandler,
		NotificationPreferenceHandler:    preferenceHandler,
		NotificationTemplateHandler:      templateHandler,
		NotificationVariableHandler:      variableHandler,
		PermissionHandler:                permHandler,
		PlatformProductHandler:           platformProductHandler,
		PublicProductHandler:             publicProductHandler,
		PlatformSubscriptionHandler:      platformSubscriptionHandler,
		PlatformBillingHandler:           platformBillingHandler,
		TenantBillingHandler:             tenantBillingHandler,
		DokuWebhookHandler:               dokuWebhookHandler,
		OrganizationDomainHandler:        organizationDomainHandler,
		OrganizationEntitlementHandler:   organizationEntitlementHandler,
		OrganizationImpersonationHandler: organizationImpersonationHandler,
		OrganizationOnboardingHandler:    organizationOnboardingHandler,
		OrganizationPlatformHandler:      organizationPlatformHandler,
		OrganizationSelfHandler:          organizationSelfHandler,
		OrganizationSwitchHandler:        organizationSwitchHandler,
		UserAuthHandler:                  authHandler,
		UserHandler:                      userHandler,
		LandingAdminPageHandler:          landingAdminPageHandler,
		LandingAdminSectionHandler:       landingAdminSectionHandler,
		LandingAdminBrandingHandler:      landingAdminBrandingHandler,
		LandingAdminDomainHandler:        landingAdminDomainHandler,
		LandingAdminFormHandler:          landingAdminFormHandler,
		LandingAdminSubmissionHandler:    landingAdminSubmissionHandler,
		PublicLandingHandler:             publicLandingHandler,
		LandingAdminCTAHandler:           landingAdminCTAHandler,
		LandingAdminTemplateHandler:      landingAdminTemplateHandler,
		LandingAdminMediaHandler:         landingAdminMediaHandler,
		MediaStorage:                     mediaStorage,
		LandingAdminNavigationHandler:    landingAdminNavigationHandler,
		LandingAdminRevisionHandler:      landingAdminRevisionHandler,
		LandingAdminScheduleHandler:      landingAdminScheduleHandler,
		LandingAdminIntegrationHandler:   landingAdminIntegrationHandler,
		PermissionChecker:                permService,
		Authenticator:                    authService,
		OrganizationResolver:             organizationResolver,
		PublicHostResolver:               publicHostResolver,
	})
	if err != nil {
		return nil, fmt.Errorf("configure trusted proxies: %w", err)
	}

	server := &http.Server{
		Addr:         cfg.App.Address(),
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.HTTP.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.HTTP.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:  time.Duration(cfg.HTTP.IdleTimeoutSeconds) * time.Second,
	}

	return &App{
		config: cfg,
		logger: log,
		db:     db,
		redis:  redisClient,
		router: router,
		server: server,
	}, nil
}

func Run(ctx context.Context) error {
	app, err := New(ctx)
	if err != nil {
		return err
	}
	app.logger.Info("starting server", "addr", app.server.Addr, "env", app.config.App.Env, "version", app.config.App.Version)
	return app.serve(ctx)
}

func (a *App) cleanup() {
	if a.redis != nil {
		_ = a.redis.Close()
	}
	if a.db != nil {
		a.db.Close()
	}
}
