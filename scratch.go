//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()
	connStr := "postgres://yulianto:@22Nov20@zyad.cloud:5432/platform?sslmode=disable"
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback(ctx)

	// Add policy
	_, err = tx.Exec(ctx, "CREATE POLICY test_public_access ON landing_pages AS PERMISSIVE FOR SELECT TO PUBLIC USING (organization_id IS NULL);")
	if err != nil {
		// Ignore if exists
	}

	// Simulate RLS
	_, err = tx.Exec(ctx, "SELECT set_config('app.organization_id', '', true)")
	if err != nil {
		log.Fatal(err)
	}

	var count int
	err = tx.QueryRow(ctx, "SELECT COUNT(*) FROM landing_pages WHERE slug = 'public-marketing' AND deleted_at IS NULL").Scan(&count)

	// Drop policy
	_, _ = tx.Exec(ctx, "DROP POLICY IF EXISTS test_public_access ON landing_pages")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Count: %d\n", count)
}
