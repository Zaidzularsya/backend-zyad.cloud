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
	userseeder "zyad.cloud/internal/modules/user/seeder"
	"zyad.cloud/internal/platform/database"
)

func main() {
	name := flag.String("name", "", "seed name to run")
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

	switch *name {
	case "super-admin":
		result, err := userseeder.SeedSuperAdmin(ctx, db, cfg.SeedAdmin)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				logger.Info("seed cancelled")
				return
			}
			logger.Error("seed failed", "name", *name, "error", err)
			os.Exit(1)
		}
		fmt.Printf("Seeded super-admin user_id=%s role_id=%s\n", result.UserID, result.RoleID)
	case "":
		logger.Error("seed name is required")
		os.Exit(1)
	default:
		logger.Error("unsupported seed", "name", *name)
		os.Exit(1)
	}
}
