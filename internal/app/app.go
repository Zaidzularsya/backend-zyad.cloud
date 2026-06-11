package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"zyad.cloud/internal/config"
	permissionhandler "zyad.cloud/internal/core/permission/handler"
	permissionrepo "zyad.cloud/internal/core/permission/repository"
	permissionservice "zyad.cloud/internal/core/permission/service"
	userhandler "zyad.cloud/internal/modules/user/handler"
	userrepo "zyad.cloud/internal/modules/user/repository"
	userservice "zyad.cloud/internal/modules/user/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/logger"
	redisplatform "zyad.cloud/internal/platform/redis"
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

	authRepo := userrepo.NewAuthRepository(db)
	authService, err := userservice.NewAuthService(authRepo, cfg)
	if err != nil {
		return nil, fmt.Errorf("create auth service: %w", err)
	}
	authHandler := userhandler.NewAuthHandler(authService)

	router := newRouter(Dependencies{
		Config:            cfg,
		Logger:            log,
		DB:                db,
		Redis:             redisClient,
		PermissionHandler: permHandler,
		UserAuthHandler:   authHandler,
	})

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
