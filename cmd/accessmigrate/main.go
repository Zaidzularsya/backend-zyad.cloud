// Command accessmigrate menjalankan migrasi akses R4-S5 (pensiun plan/subscription/billing lama).
//
//	accessmigrate preflight          # laporan; exit 1 bila masih ada data berbayar lama
//	accessmigrate backfill-default   # beri akses default/contract lalu expire entitlement `plan`
//
// Keduanya idempoten dan harus dijalankan sebelum migration 000154.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"zyad.cloud/internal/app"
	"zyad.cloud/internal/config"
	"zyad.cloud/internal/platform/database"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: accessmigrate preflight|backfill-default")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Load()
	db, err := database.Connect(ctx, cfg.Database, logger)
	if err != nil {
		logger.Error("failed to connect database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	switch os.Args[1] {
	case "preflight":
		report, err := app.RunPreflight(ctx, db)
		if err != nil {
			logger.Error("preflight failed", "error", err)
			os.Exit(1)
		}
		fmt.Printf("paid_active_subscriptions=%d open_legacy_invoices=%d workspaces_needing_backfill=%d\n",
			report.PaidActiveSubscriptions, report.OpenLegacyInvoices, report.WorkspacesNeedingBackfill)
		for _, line := range report.Details {
			fmt.Println("  -", line)
		}
		if !report.Clean() {
			fmt.Println("NOT CLEAN: pindahkan data berbayar lama ke contract secara manual sebelum migration 000154")
			os.Exit(1)
		}
	case "backfill-default":
		result, err := app.RunBackfillDefaultFromConfig(ctx, cfg, db)
		if err != nil {
			logger.Error("backfill failed", "error", err)
			os.Exit(1)
		}
		fmt.Printf("granted=%d skipped_contract=%d expired_plan_rows=%d\n",
			result.Granted, result.SkippedContract, result.ExpiredPlanRows)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
		os.Exit(2)
	}
}
