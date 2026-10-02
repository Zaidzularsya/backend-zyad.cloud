package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
)

// withScopedTx menjalankan fn dalam transaksi dengan app.organization_id
// diset ke scope (RLS), untuk repository tanpa struct withTx sendiri.
func withScopedTx(ctx context.Context, db *database.Pool, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID()); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
