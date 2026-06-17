package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type revisionRepository struct {
	db *database.Pool
}

func NewRevisionRepository(db *database.Pool) RevisionRepository {
	return &revisionRepository{db: db}
}

func (r *revisionRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

func (r *revisionRepository) CreateRevision(ctx context.Context, scope coretenant.Scope, params CreateRevisionParams) (domain.LandingPageRevision, error) {
	if !scope.IsValid() {
		return domain.LandingPageRevision{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_page_revisions (
			organization_id, landing_page_id, revision_number, snapshot, change_note, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6
		) RETURNING
			id, landing_page_id, revision_number, snapshot, change_note, created_by, created_at
	`

	var rev domain.LandingPageRevision
	var createdBy, changeNote *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var cb interface{} = nil
		if params.CreatedBy != "" {
			cb = params.CreatedBy
		}
		var cn interface{} = nil
		if params.ChangeNote != "" {
			cn = params.ChangeNote
		}

		err := tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.LandingPageID,
			params.RevisionNumber,
			params.Snapshot,
			cn,
			cb,
		).Scan(
			&rev.ID, &rev.LandingPageID, &rev.RevisionNumber, &rev.Snapshot,
			&changeNote, &createdBy, &rev.CreatedAt,
		)
		return err
	})

	if err != nil {
		return domain.LandingPageRevision{}, err
	}
	rev.OrganizationID = scope.OrganizationID()
	if createdBy != nil { rev.CreatedBy = *createdBy }
	if changeNote != nil { rev.ChangeNote = *changeNote }

	return rev, nil
}

func (r *revisionRepository) GetRevision(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingPageRevision, error) {
	if !scope.IsValid() {
		return domain.LandingPageRevision{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, revision_number, snapshot, change_note, created_by, created_at
		FROM landing_page_revisions
		WHERE id = $1 AND organization_id = $2
	`

	var rev domain.LandingPageRevision
	var createdBy, changeNote *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&rev.ID, &rev.LandingPageID, &rev.RevisionNumber, &rev.Snapshot,
			&changeNote, &createdBy, &rev.CreatedAt,
		)
	})

	if err != nil {
		return domain.LandingPageRevision{}, err
	}
	rev.OrganizationID = scope.OrganizationID()
	if createdBy != nil { rev.CreatedBy = *createdBy }
	if changeNote != nil { rev.ChangeNote = *changeNote }

	return rev, nil
}

func (r *revisionRepository) ListRevisions(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingPageRevision, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, revision_number, snapshot, change_note, created_by, created_at
		FROM landing_page_revisions
		WHERE organization_id = $1 AND landing_page_id = $2
		ORDER BY revision_number DESC
	`

	var revisions []domain.LandingPageRevision

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID(), pageID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var rev domain.LandingPageRevision
			var createdBy, changeNote *string

			err := rows.Scan(
				&rev.ID, &rev.LandingPageID, &rev.RevisionNumber, &rev.Snapshot,
				&changeNote, &createdBy, &rev.CreatedAt,
			)
			if err != nil {
				return err
			}
			rev.OrganizationID = scope.OrganizationID()
			if createdBy != nil { rev.CreatedBy = *createdBy }
			if changeNote != nil { rev.ChangeNote = *changeNote }
			revisions = append(revisions, rev)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return revisions, nil
}

func (r *revisionRepository) GetLatestRevision(ctx context.Context, scope coretenant.Scope, pageID string) (domain.LandingPageRevision, error) {
	if !scope.IsValid() {
		return domain.LandingPageRevision{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, revision_number, snapshot, change_note, created_by, created_at
		FROM landing_page_revisions
		WHERE organization_id = $1 AND landing_page_id = $2
		ORDER BY revision_number DESC
		LIMIT 1
	`

	var rev domain.LandingPageRevision
	var createdBy, changeNote *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, scope.OrganizationID(), pageID).Scan(
			&rev.ID, &rev.LandingPageID, &rev.RevisionNumber, &rev.Snapshot,
			&changeNote, &createdBy, &rev.CreatedAt,
		)
	})

	if err != nil {
		return domain.LandingPageRevision{}, err
	}
	rev.OrganizationID = scope.OrganizationID()
	if createdBy != nil { rev.CreatedBy = *createdBy }
	if changeNote != nil { rev.ChangeNote = *changeNote }

	return rev, nil
}

func (r *revisionRepository) CreateSchedule(ctx context.Context, scope coretenant.Scope, params CreateScheduleParams) (domain.LandingPageSchedule, error) {
	if !scope.IsValid() {
		return domain.LandingPageSchedule{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_page_schedules (
			organization_id, landing_page_id, action, scheduled_at, created_by
		) VALUES (
			$1, $2, $3, $4, $5
		) RETURNING
			id, landing_page_id, action, scheduled_at, status, lock_id, lock_expires_at, error_message, attempts, created_by, created_at, updated_at
	`

	var s domain.LandingPageSchedule
	var lockID, createdBy, errorMessage *string
	var lockExpiresAt *time.Time

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var cb interface{} = nil
		if params.CreatedBy != "" {
			cb = params.CreatedBy
		}

		err := tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.LandingPageID,
			params.Action,
			params.ScheduledAt,
			cb,
		).Scan(
			&s.ID, &s.LandingPageID, &s.Action, &s.ScheduledAt, &s.Status,
			&lockID, &lockExpiresAt, &errorMessage, &s.Attempts, &createdBy,
			&s.CreatedAt, &s.UpdatedAt,
		)
		return err
	})

	if err != nil {
		return domain.LandingPageSchedule{}, err
	}
	s.OrganizationID = scope.OrganizationID()
	if lockID != nil { s.LockID = lockID }
	if lockExpiresAt != nil { s.LockExpiresAt = lockExpiresAt }
	if errorMessage != nil { s.ErrorMessage = *errorMessage }
	if createdBy != nil { s.CreatedBy = *createdBy }

	return s, nil
}

func (r *revisionRepository) GetSchedule(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingPageSchedule, error) {
	if !scope.IsValid() {
		return domain.LandingPageSchedule{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, action, scheduled_at, status, lock_id, lock_expires_at, error_message, attempts, created_by, created_at, updated_at
		FROM landing_page_schedules
		WHERE id = $1 AND organization_id = $2
	`

	var s domain.LandingPageSchedule
	var lockID, createdBy, errorMessage *string
	var lockExpiresAt *time.Time

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&s.ID, &s.LandingPageID, &s.Action, &s.ScheduledAt, &s.Status,
			&lockID, &lockExpiresAt, &errorMessage, &s.Attempts, &createdBy,
			&s.CreatedAt, &s.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingPageSchedule{}, err
	}
	s.OrganizationID = scope.OrganizationID()
	if lockID != nil { s.LockID = lockID }
	if lockExpiresAt != nil { s.LockExpiresAt = lockExpiresAt }
	if errorMessage != nil { s.ErrorMessage = *errorMessage }
	if createdBy != nil { s.CreatedBy = *createdBy }

	return s, nil
}

func (r *revisionRepository) ListSchedules(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingPageSchedule, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, action, scheduled_at, status, lock_id, lock_expires_at, error_message, attempts, created_by, created_at, updated_at
		FROM landing_page_schedules
		WHERE organization_id = $1 AND landing_page_id = $2
		ORDER BY scheduled_at DESC
	`

	var schedules []domain.LandingPageSchedule

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID(), pageID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s domain.LandingPageSchedule
			var lockID, createdBy, errorMessage *string
			var lockExpiresAt *time.Time

			err := rows.Scan(
				&s.ID, &s.LandingPageID, &s.Action, &s.ScheduledAt, &s.Status,
				&lockID, &lockExpiresAt, &errorMessage, &s.Attempts, &createdBy,
				&s.CreatedAt, &s.UpdatedAt,
			)
			if err != nil {
				return err
			}
			s.OrganizationID = scope.OrganizationID()
			if lockID != nil { s.LockID = lockID }
			if lockExpiresAt != nil { s.LockExpiresAt = lockExpiresAt }
			if errorMessage != nil { s.ErrorMessage = *errorMessage }
			if createdBy != nil { s.CreatedBy = *createdBy }
			schedules = append(schedules, s)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return schedules, nil
}

func (r *revisionRepository) DeleteSchedule(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		DELETE FROM landing_page_schedules
		WHERE id = $1 AND organization_id = $2 AND status = 'pending'
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *revisionRepository) ClaimPendingSchedules(ctx context.Context, limit int, lockDuration time.Duration) ([]domain.LandingPageSchedule, error) {
	// Worker operation, runs outside tenant scope (system-level)
	query := `
		WITH pending AS (
			SELECT id
			FROM landing_page_schedules
			WHERE (status = 'pending' OR status = 'failed')
			  AND scheduled_at <= NOW()
			  AND (lock_expires_at IS NULL OR lock_expires_at <= NOW())
			ORDER BY scheduled_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE landing_page_schedules s
		SET status = 'processing',
			lock_id = gen_random_uuid(),
			lock_expires_at = NOW() + $2::interval,
			attempts = attempts + 1,
			updated_at = NOW()
		FROM pending p
		WHERE s.id = p.id
		RETURNING s.id, s.organization_id, s.landing_page_id, s.action, s.scheduled_at, s.status, s.lock_id, s.lock_expires_at, s.error_message, s.attempts, s.created_by, s.created_at, s.updated_at
	`

	var schedules []domain.LandingPageSchedule

	// Temporarily bypass RLS if possible, or we could test with a transaction and a scope?
	// But ClaimPendingSchedules doesn't take a scope.
	rows, err := r.db.Query(ctx, query, limit, lockDuration)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var s domain.LandingPageSchedule
		var retLockID, createdBy, errorMessage *string
		var lockExpiresAt *time.Time

		err := rows.Scan(
			&s.ID, &s.OrganizationID, &s.LandingPageID, &s.Action, &s.ScheduledAt, &s.Status,
			&retLockID, &lockExpiresAt, &errorMessage, &s.Attempts, &createdBy,
			&s.CreatedAt, &s.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if retLockID != nil { s.LockID = retLockID }
		if lockExpiresAt != nil { s.LockExpiresAt = lockExpiresAt }
		if errorMessage != nil { s.ErrorMessage = *errorMessage }
		if createdBy != nil { s.CreatedBy = *createdBy }
		schedules = append(schedules, s)
	}

	return schedules, rows.Err()
}

func (r *revisionRepository) MarkScheduleStatus(ctx context.Context, id string, status domain.ScheduleStatus, errorMessage *string) error {
	// Worker operation, runs outside tenant scope
	query := `
		UPDATE landing_page_schedules
		SET status = $1,
			error_message = $2,
			lock_id = NULL,
			lock_expires_at = NULL,
			updated_at = NOW()
		WHERE id = $3
	`

	cmdTag, err := r.db.Exec(ctx, query, status, errorMessage, id)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
