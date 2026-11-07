//go:build integration

package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/mikrotik/domain"
)

// ListSecretsInTx runs List's SQL (without opening or committing a transaction)
// so integration tests can execute it inside a rolled-back transaction.
func ListSecretsInTx(ctx context.Context, tx pgx.Tx, orgID, routerID string, limit, offset int) ([]domain.PPPSecret, int64, error) {
	cond := "s.organization_id = $1 AND s.router_id = $2"
	return listSecretsTx(ctx, tx, cond, []interface{}{orgID, routerID}, limit, offset)
}

// ActivityListSQL exposes the activity page query to EXPLAIN tests.
const ActivityListSQL = activityListSQL
