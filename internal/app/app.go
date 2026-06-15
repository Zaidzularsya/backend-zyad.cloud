package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"zyad.cloud/internal/config"
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
	organizationhandler "zyad.cloud/internal/modules/organization/handler"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	userhandler "zyad.cloud/internal/modules/user/handler"
	userrepo "zyad.cloud/internal/modules/user/repository"
	userservice "zyad.cloud/internal/modules/user/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/logger"
	"zyad.cloud/internal/platform/mail"
	redisplatform "zyad.cloud/internal/platform/redis"
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
	authService.SetNotificationPublisher(notificationpublisher.NewOutboxPublisher(outboxRepo, cfg.Notification.MaxAttempts))
	authHandler := userhandler.NewAuthHandler(authService)
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

	router, err := newRouter(Dependencies{
		Config:                        cfg,
		Logger:                        log,
		DB:                            db,
		Redis:                         redisClient,
		NotificationHandler:           notificationHandler,
		NotificationLogHandler:        logHandler,
		NotificationPreferenceHandler: preferenceHandler,
		NotificationTemplateHandler:   templateHandler,
		NotificationVariableHandler:   variableHandler,
		PermissionHandler:             permHandler,
		OrganizationSwitchHandler:     organizationSwitchHandler,
		UserAuthHandler:               authHandler,
		UserHandler:                   userHandler,
		Authenticator:                 authService,
		OrganizationResolver:          organizationResolver,
		PublicHostResolver:            publicHostResolver,
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
