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

	"zyad.cloud/internal/app"
	"zyad.cloud/internal/config"
	notificationconsumer "zyad.cloud/internal/core/notification/consumer"
	notificationdispatcher "zyad.cloud/internal/core/notification/dispatcher"
	notificationrepo "zyad.cloud/internal/core/notification/repository"
	notificationservice "zyad.cloud/internal/core/notification/service"
	notificationtemplate "zyad.cloud/internal/core/notification/template"
	mailboxservice "zyad.cloud/internal/modules/mailbox/service"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	receivableservice "zyad.cloud/internal/modules/receivable/service"
	whatsapprealtime "zyad.cloud/internal/modules/whatsapp/realtime"
	whatsapprepo "zyad.cloud/internal/modules/whatsapp/repository"
	whatsappservice "zyad.cloud/internal/modules/whatsapp/service"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/logger"
	"zyad.cloud/internal/platform/mail"
	redisplatform "zyad.cloud/internal/platform/redis"
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

	notificationService := buildNotificationService(db, cfg, log)
	worker := buildNotificationWorker(db, notificationService)

	batchSize := cfg.Notification.WorkerBatchSize
	if batchSize <= 0 {
		batchSize = 20
	}

	reconciler := buildWhatsAppReconciler(db, cfg, log)
	// Redis only carries realtime hints to the API's SSE streams; the worker
	// works without it (clients fall back to polling).
	var whatsappBus *whatsapprealtime.Bus
	if redisClient, redisErr := redisplatform.Connect(ctx, cfg.Redis, log); redisErr != nil {
		log.Warn("redis unavailable, whatsapp realtime disabled", "error", redisErr)
	} else {
		defer redisClient.Close()
		whatsappBus = app.NewWhatsAppRealtimeBus(redisClient, log)
	}
	inbound := app.NewWhatsAppInboundProcessor(cfg, db, whatsappBus, log)
	mailSync := app.NewMailSyncService(cfg, db, log)
	overdue := app.NewReceivableOverdueRunner(db)
	billingRun, err := app.NewReceivableBillingRunner(cfg, db, log)
	if err != nil {
		log.Warn("receivable billing run disabled", "error", err)
	}

	if *once {
		if err := runBatch(ctx, log, worker, notificationService, batchSize); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("worker batch failed", "error", err)
			os.Exit(1)
		}
		if inbound != nil {
			runWhatsAppInbound(ctx, log, inbound)
		}
		if reconciler != nil {
			runWhatsAppReconcile(ctx, log, reconciler)
		}
		if mailSync != nil {
			runMailSync(ctx, log, mailSync)
		}
		runReceivableOverdue(ctx, log, overdue)
		if billingRun != nil {
			runReceivableBillingRun(ctx, log, billingRun)
		}
		return
	}

	if inbound != nil {
		inboundInterval := time.Duration(cfg.WhatsApp.WorkerIntervalSeconds) * time.Second
		if inboundInterval <= 0 {
			inboundInterval = 5 * time.Second
		}
		log.Info("starting whatsapp inbound processor", "interval", inboundInterval.String())
		go runEvery(ctx, inboundInterval, func() { runWhatsAppInbound(ctx, log, inbound) })
	}

	if reconciler != nil {
		reconcileInterval := time.Duration(cfg.WhatsApp.ReconcileIntervalSeconds) * time.Second
		if reconcileInterval <= 0 {
			reconcileInterval = 5 * time.Minute
		}
		log.Info("starting whatsapp session reconciler", "interval", reconcileInterval.String())
		go runEvery(ctx, reconcileInterval, func() { runWhatsAppReconcile(ctx, log, reconciler) })
	}

	if mailSync != nil {
		syncInterval := time.Duration(cfg.Mail.MailSyncIntervalSeconds) * time.Second
		if syncInterval <= 0 {
			syncInterval = 5 * time.Minute
		}
		log.Info("starting mail sync", "interval", syncInterval.String())
		go runEvery(ctx, syncInterval, func() { runMailSync(ctx, log, mailSync) })
	}

	log.Info("starting receivable overdue job", "interval", receivableOverdueInterval.String())
	go runEvery(ctx, receivableOverdueInterval, func() { runReceivableOverdue(ctx, log, overdue) })

	if billingRun != nil {
		log.Info("starting receivable billing run job", "interval", receivableBillingRunInterval.String())
		go runEvery(ctx, receivableBillingRunInterval, func() { runReceivableBillingRun(ctx, log, billingRun) })
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

func buildNotificationService(db *database.Pool, cfg config.Config, log *slog.Logger) *notificationservice.NotificationService {
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
		notificationdispatcher.NewWhatsAppDispatcher(app.NewWhatsAppNotificationClient(cfg, db, log)),
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

// buildWhatsAppReconciler returns nil when WAHA is not configured, so the
// worker keeps running notification batches only.
func buildWhatsAppReconciler(db *database.Pool, cfg config.Config, log *slog.Logger) *whatsappservice.StatusReconciler {
	if app.NewWAHAProvider(cfg.WhatsApp) == nil {
		return nil
	}
	return whatsappservice.NewStatusReconciler(
		whatsapprepo.NewDirectoryRepository(db),
		app.NewWhatsAppWorkerResolver(db),
		app.NewWhatsAppSessionService(cfg, db, nil, log),
		log,
	)
}

func runWhatsAppReconcile(ctx context.Context, log *slog.Logger, reconciler *whatsappservice.StatusReconciler) {
	result, err := reconciler.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error("whatsapp reconcile failed", "error", err)
		return
	}
	if result.Checked > 0 {
		log.Info("reconciled whatsapp sessions", "checked", result.Checked, "failed", result.Failed)
	}
}

const whatsappInboundBatchSize = 50

func runWhatsAppInbound(ctx context.Context, log *slog.Logger, processor *whatsappservice.InboundProcessor) {
	result, err := processor.RunOnce(ctx, whatsappInboundBatchSize)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error("whatsapp inbound batch failed", "error", err)
		return
	}
	if result.Claimed > 0 {
		log.Info("processed whatsapp webhook events", "claimed", result.Claimed, "processed", result.Processed, "failed", result.Failed)
	}
}

// receivableOverdueInterval: jatuh tempo dihitung per hari WIB, jadi cek per jam sudah cukup.
const receivableOverdueInterval = time.Hour

func runReceivableOverdue(ctx context.Context, log *slog.Logger, runner *receivableservice.OverdueRunner) {
	result, err := runner.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error("receivable overdue run failed", "error", err)
		return
	}
	if result.Marked > 0 || result.Failed > 0 {
		log.Info("marked overdue invoices", "organizations", result.Checked, "marked", result.Marked, "failed", result.Failed)
	}
}

// receivableBillingRunInterval: tanggal tagih dihitung per hari WIB; per jam cukup, dan run idempoten.
const receivableBillingRunInterval = time.Hour

func runReceivableBillingRun(ctx context.Context, log *slog.Logger, runner *receivableservice.BillingRunner) {
	result, err := runner.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error("receivable billing run failed", "error", err)
		return
	}
	log.Info("receivable billing run", "organizations", result.Checked, "invoices", result.Invoices,
		"advanced", result.Advanced, "ended", result.Ended, "failed", result.Failed)
}

func runMailSync(ctx context.Context, log *slog.Logger, sync *mailboxservice.SyncService) {
	result, err := sync.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error("mail sync failed", "error", err)
		return
	}
	if result.Checked > 0 {
		log.Info("synced mailboxes", "checked", result.Checked, "synced", result.Synced, "failed", result.Failed)
	}
}

// runEvery calls fn immediately and then on every tick until ctx is done.
func runEvery(ctx context.Context, interval time.Duration, fn func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		fn()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
