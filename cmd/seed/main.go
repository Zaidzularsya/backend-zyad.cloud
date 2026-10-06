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
	crmseeder "zyad.cloud/internal/modules/crm/seeder"
	organizationseeder "zyad.cloud/internal/modules/organization/seeder"
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
	case "platform-organization":
		result, err := organizationseeder.SeedPlatformOrganization(
			ctx,
			db,
			cfg.MultiTenant,
			cfg.SeedAdmin,
		)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				logger.Info("seed cancelled")
				return
			}
			logger.Error("seed failed", "name", *name, "error", err)
			os.Exit(1)
		}
		fmt.Printf(
			"Seeded platform-organization organization_id=%s owner_user_id=%s membership_id=%s entitlement_id=%s domain_ids=%v\n",
			result.OrganizationID,
			result.OwnerUserID,
			result.MembershipID,
			result.EntitlementID,
			result.DomainIDs,
		)
	case "self-serve":
		var platformID string
		if err := db.QueryRow(ctx, `SELECT id FROM organizations WHERE type = 'platform' AND deleted_at IS NULL LIMIT 1`).Scan(&platformID); err != nil {
			logger.Error("platform organization not found; run seed platform-organization first", "error", err)
			os.Exit(1)
		}
		result, err := crmseeder.SeedSelfServe(ctx, db, platformID, cfg.SelfServe.BotEmail)
		if err != nil {
			logger.Error("seed failed", "name", *name, "error", err)
			os.Exit(1)
		}
		fmt.Printf("Seeded self-serve bot_user_id=%s pipeline_id=%s\n", result.BotUserID, result.PipelineID)
	case "":
		logger.Error("seed name is required")
		os.Exit(1)
	default:
		logger.Error("unsupported seed", "name", *name)
		os.Exit(1)
	}
}
