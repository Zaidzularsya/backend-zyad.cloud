package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"zyad.cloud/internal/config"
	notificationconsumer "zyad.cloud/internal/core/notification/consumer"
	notificationdispatcher "zyad.cloud/internal/core/notification/dispatcher"
	notificationrepo "zyad.cloud/internal/core/notification/repository"
	notificationservice "zyad.cloud/internal/core/notification/service"
	notificationtemplate "zyad.cloud/internal/core/notification/template"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/logger"
	"zyad.cloud/internal/platform/mail"
)

func main() {
	once := flag.Bool("once", false, "run one worker batch then exit")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	log := logger.New(cfg.App.Env)

	db, err := database.Connect(ctx, cfg.Database, log)
	if err != nil {
		log.Error("failed to connect database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	notificationService := buildNotificationService(db, cfg)
	worker := buildNotificationWorker(db, notificationService)

	batchSize := cfg.Notification.WorkerBatchSize
	if batchSize <= 0 {
		batchSize = 20
	}

	if *once {
		if err := runBatch(ctx, log, worker, notificationService, batchSize); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("worker batch failed", "error", err)
			os.Exit(1)
		}
		return
	}

	interval := time.Duration(cfg.Notification.WorkerIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}

	log.Info("starting notification worker", "interval", interval.String(), "batch_size", batchSize)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := runBatch(ctx, log, worker, notificationService, batchSize); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("worker batch failed", "error", err)
		}

		select {
		case <-ctx.Done():
			log.Info("notification worker stopped")
			return
		case <-ticker.C:
		}
	}
}

func buildNotificationWorker(db *database.Pool, notificationService *notificationservice.NotificationService) *notificationconsumer.OutboxWorker {
	ruleService := notificationservice.NewNotificationRuleService()
	eventConsumer := notificationconsumer.NewNotificationEventConsumer(ruleService, notificationService)
	outboxRepo := notificationrepo.NewOutboxRepository(db)
	worker := notificationconsumer.NewOutboxWorker(outboxRepo, eventConsumer)
	worker.SetTenantResolver(
		organizationservice.NewWorkerResolver(
			organizationrepo.NewOrganizationRepository(db),
			"notification-worker",
		),
		"notification-worker",
	)
	return worker
}

func buildNotificationService(db *database.Pool, cfg config.Config) *notificationservice.NotificationService {
	templateRepo := notificationrepo.NewTemplateRepository(db)
	logRepo := notificationrepo.NewNotificationLogRepository(db)
	preferenceRepo := notificationrepo.NewPreferenceRepository(db)
	renderer := notificationtemplate.NewRenderer(notificationtemplate.NewVariableRegistry())

	return notificationservice.NewNotificationService(
		templateRepo,
		logRepo,
		preferenceRepo,
		renderer,
		notificationdispatcher.NewEmailDispatcher(mail.NewMailerFromConfig(cfg.Mail)),
	)
}

func runBatch(
	ctx context.Context,
	log *slog.Logger,
	worker *notificationconsumer.OutboxWorker,
	notificationService *notificationservice.NotificationService,
	batchSize int,
) error {
	results, err := worker.RunOnce(ctx, batchSize)
	if err != nil {
		return err
	}
	if len(results) > 0 {
		log.Info("processed notification outbox events", "count", len(results))
	}

	retried, err := notificationService.RetryDue(ctx, batchSize)
	if err != nil {
		return err
	}
	if len(retried) > 0 {
		log.Info("processed notification retry logs", "count", len(retried))
	}

	return nil
}
