//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := "postgres://yulianto@localhost:5432/platform?sslmode=disable"
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SELECT set_config('app.organization_id', '', true)")
	if err != nil {
		log.Fatal(err)
	}

	var id string
	err = tx.QueryRow(ctx, "SELECT id FROM landing_pages WHERE slug = 'public-marketing' AND deleted_at IS NULL").Scan(&id)
	if err != nil {
		fmt.Println("Query Error:", err)
		os.Exit(1)
	}

	fmt.Println("Found ID:", id)
}
