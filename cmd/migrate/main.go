package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/migration"
)

func main() {
	direction := flag.String("direction", string(migration.DirectionUp), "migration direction: up or down")
	dir := flag.String("dir", "migrations", "migration directory")
	steps := flag.Int("steps", 0, "maximum migrations to run, 0 means all pending")
	flag.Parse()

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

	results, err := migration.Run(ctx, db, *dir, migration.Direction(*direction), *steps)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Info("migration cancelled")
			return
		}
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Println("No migrations to run.")
		return
	}

	for _, result := range results {
		fmt.Printf("Applied %s_%s\n", result.Version, result.Name)
	}
}
