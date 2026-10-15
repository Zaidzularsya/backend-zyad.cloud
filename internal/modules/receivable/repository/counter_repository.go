package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
)

type counterRepository struct{ db *database.Pool }

func NewCounterRepository(db *database.Pool) CounterRepository { return &counterRepository{db: db} }

func (r *counterRepository) Next(ctx context.Context, scope coretenant.Scope, documentType string, year int) (int, error) {
	var n int
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO receivable_document_counters (organization_id, document_type, year, last_number)
			VALUES ($1, $2, $3, 1)
			ON CONFLICT (organization_id, document_type, year)
			DO UPDATE SET last_number = receivable_document_counters.last_number + 1
			RETURNING last_number`, scope.OrganizationID(), documentType, year).Scan(&n)
	})
	return n, err
}
