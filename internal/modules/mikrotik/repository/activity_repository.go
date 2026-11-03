package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
)

// ActivityRow is one audit row of the control center log. SecretName and
// SecretAccountID come from the secret named by metadata.secret_id (empty when
// the secret is gone or the event has none).
type ActivityRow struct {
	ID              string
	At              time.Time
	Event           string
	ActorUserID     string
	TargetID        string
	Metadata        map[string]any
	SecretName      string
	SecretAccountID string
}

// ActivityRepository reads the mikrotik audit trail of one organization.
type ActivityRepository interface {
	// ListActivity pages audit rows of the given events, newest first, and
	// returns the total number of matching rows.
	ListActivity(ctx context.Context, scope coretenant.Scope, events []string, limit, offset int) ([]ActivityRow, int64, error)
}

// activityListSQL is the page query; ActivityListSQL exposes it to the plan test.
const activityListSQL = `
			SELECT a.id::text, a.created_at, a.event, COALESCE(a.actor_user_id::text, ''), COALESCE(a.target_id::text, ''),
			       a.metadata, COALESCE(s.name, ''), COALESCE(s.account_id::text, '')
			FROM audit_logs a
			LEFT JOIN mikrotik_ppp_secrets s
			  ON s.organization_id = a.organization_id
			 AND s.id = CASE WHEN a.metadata->>'secret_id' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
			                 THEN (a.metadata->>'secret_id')::uuid END
			WHERE a.organization_id = $1 AND a.module = 'mikrotik' AND a.event = ANY($2)
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $3 OFFSET $4`

type activityRepository struct{ db *database.Pool }

func NewActivityRepository(db *database.Pool) ActivityRepository { return &activityRepository{db: db} }

func (r *activityRepository) ListActivity(ctx context.Context, scope coretenant.Scope, events []string, limit, offset int) ([]ActivityRow, int64, error) {
	limit = clampLimit(limit)
	if offset < 0 {
		offset = 0
	}
	out := []ActivityRow{}
	var total int64
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM audit_logs
			WHERE organization_id = $1 AND module = 'mikrotik' AND event = ANY($2)`,
			scope.OrganizationID(), events).Scan(&total); err != nil {
			return err
		}
		// metadata.secret_id is only joined when it looks like a uuid, so a
		// malformed value can never fail the cast (the CASE guards evaluation).
		rows, err := tx.Query(ctx, activityListSQL,
			scope.OrganizationID(), events, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row ActivityRow
			// Scanned as any: a row whose jsonb is not an object (array,
			// scalar) must not fail the whole page.
			var md any
			if err := rows.Scan(&row.ID, &row.At, &row.Event, &row.ActorUserID, &row.TargetID,
				&md, &row.SecretName, &row.SecretAccountID); err != nil {
				return err
			}
			row.Metadata, _ = md.(map[string]any)
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
