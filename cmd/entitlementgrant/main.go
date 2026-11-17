// Command entitlementgrant grants a platform_override entitlement with a
// UTC-correct window. Usage:
//
//	go run ./cmd/entitlementgrant --org <uuid> --feature mikrotik.enabled --days 90 --reason "pilot" --by <user uuid>
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/platform/database"
)

func main() {
	in := grantInput{}
	flag.StringVar(&in.OrganizationID, "org", "", "organization uuid")
	flag.StringVar(&in.FeatureKey, "feature", "", "feature key, e.g. mikrotik.enabled")
	flag.IntVar(&in.Days, "days", 0, "how many days the grant lasts")
	flag.StringVar(&in.Reason, "reason", "", "why (stored in the audit column)")
	flag.StringVar(&in.ActorUserID, "by", "", "uuid of the user granting this")
	flag.Parse()

	// Fail on bad input before opening the database.
	if err := in.validate(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Connect(ctx, config.Load().Database, logger)
	if err != nil {
		logger.Error("failed to connect database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	e, err := grant(ctx, repository.NewEntitlementRepository(db), in, time.Now())
	if err != nil {
		logger.Error("grant failed", "error", err)
		os.Exit(1)
	}
	fmt.Printf("Granted %s to %s via platform_override until %s UTC (entitlement_id=%s)\n",
		in.FeatureKey, in.OrganizationID, e.EffectiveUntil.UTC().Format(time.RFC3339), e.ID)
}
