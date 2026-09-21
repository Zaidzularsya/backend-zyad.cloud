package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
)

type documentCounterRepository struct {
	db *database.Pool
}

func NewDocumentCounterRepository(db *database.Pool) DocumentCounterRepository {
	return &documentCounterRepository{db: db}
}

func (r *documentCounterRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID())
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// NextNumber atomically increments and returns the counter for the given
// document type in the current calendar year. The INSERT ... ON CONFLICT DO
// UPDATE is a single statement, so concurrent callers can never observe or
// hand out the same number twice.
func (r *documentCounterRepository) NextNumber(ctx context.Context, scope coretenant.Scope, documentType string) (int, error) {
	year := time.Now().UTC().Year()

	var lastNumber int
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO crm_document_counters (organization_id, document_type, year, last_number)
			VALUES ($1, $2, $3, 1)
			ON CONFLICT (organization_id, document_type, year)
			DO UPDATE SET last_number = crm_document_counters.last_number + 1, updated_at = now()
			RETURNING last_number
		`, scope.OrganizationID(), documentType, year).Scan(&lastNumber)
	})
	if err != nil {
		return 0, err
	}
	return lastNumber, nil
}
