//go:build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/platform/database/testutil"
)

// Every column defaulting to now() is "timestamp without time zone", so the
// wall clock it stores follows the session timezone. The pool must pin it to
// UTC, otherwise a server running in Asia/Jakarta stores WIB and the app reads
// it back as UTC (+7 hours in the future).
func TestConnectPinsSessionTimezoneToUTCIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()

	conn, err := db.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()

	var tz string
	if err := conn.QueryRow(ctx, "SHOW timezone").Scan(&tz); err != nil {
		t.Fatalf("show timezone: %v", err)
	}
	if tz != "UTC" {
		t.Fatalf("session timezone = %q, want UTC", tz)
	}

	if _, err := conn.Exec(ctx, `CREATE TEMP TABLE tz_probe (created_at timestamp without time zone NOT NULL DEFAULT now())`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO tz_probe DEFAULT VALUES`); err != nil {
		t.Fatalf("insert probe: %v", err)
	}
	var stored time.Time
	if err := conn.QueryRow(ctx, `SELECT created_at FROM tz_probe`).Scan(&stored); err != nil {
		t.Fatalf("read probe: %v", err)
	}

	if drift := time.Since(stored).Abs(); drift > 10*time.Second {
		t.Fatalf("now() default stored %s, expected within 10s of UTC now (%s); drift %s", stored, time.Now().UTC(), drift)
	}
}
