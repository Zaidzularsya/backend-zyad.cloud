//go:build integration

package testutil

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/migration"
)

func OpenTestDatabase(t *testing.T) *database.Pool {
	t.Helper()

	cfg := config.LoadTestDatabase()
	requireTestDatabase(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	db, err := database.Connect(ctx, cfg, slog.Default())
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(db.Close)

	if _, err := migration.Run(ctx, db, migrationsDir(t), migration.DirectionUp, 0); err != nil {
		t.Fatalf("run test database migrations: %v", err)
	}

	CleanupNotificationTables(t, ctx, db)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		CleanupNotificationTables(t, cleanupCtx, db)
	})

	return db
}

func CleanupNotificationTables(t *testing.T, ctx context.Context, db *database.Pool) {
	t.Helper()

	_, err := db.Exec(ctx, `
		TRUNCATE
			notification_outbox_events,
			notification_logs,
			notification_preferences
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("cleanup notification tables: %v", err)
	}

	_, err = db.Exec(ctx, `
		DELETE FROM notification_templates
		WHERE is_system = false
			OR deleted_at IS NOT NULL
	`)
	if err != nil {
		t.Fatalf("cleanup notification template test rows: %v", err)
	}
}

func UniqueCode(prefix string) string {
	return fmt.Sprintf("%s.%d", prefix, time.Now().UnixNano())
}

func requireTestDatabase(t *testing.T, cfg config.DatabaseConfig) {
	t.Helper()

	if strings.TrimSpace(cfg.Name) == "" {
		t.Fatal("TEST_DB_NAME is required for integration tests")
	}
	if !strings.Contains(strings.ToLower(cfg.Name), "test") {
		t.Fatalf("refusing to run integration cleanup against non-test database %q", cfg.Name)
	}
}

func migrationsDir(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	for {
		candidate := filepath.Join(dir, "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("migrations directory not found")
		}
		dir = parent
	}
}
