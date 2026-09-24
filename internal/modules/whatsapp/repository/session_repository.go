package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/platform/database"
)

type sessionRepository struct {
	db *database.Pool
}

func NewSessionRepository(db *database.Pool) SessionRepository {
	return &sessionRepository{db: db}
}

func (r *sessionRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

const sessionColumns = `
	id, organization_id, name, display_name, phone, push_name, status, engine, waha_server_id,
	is_default, purpose, auto_create_lead, last_status_at, created_by, updated_by,
	created_at, updated_at, deleted_at
`

func scanSession(row pgx.Row) (domain.Session, error) {
	var s domain.Session
	var displayName, phone, pushName, engine, createdBy, updatedBy *string
	var status, purpose string
	err := row.Scan(
		&s.ID, &s.OrganizationID, &s.Name, &displayName, &phone, &pushName, &status, &engine, &s.WAHAServerID,
		&s.IsDefault, &purpose, &s.AutoCreateLead, &s.LastStatusAt, &createdBy, &updatedBy,
		&s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
	)
	if err != nil {
		return domain.Session{}, err
	}
	s.Status = domain.SessionStatus(status)
	s.Purpose = domain.SessionPurpose(purpose)
	s.DisplayName = derefString(displayName)
	s.Phone = derefString(phone)
	s.PushName = derefString(pushName)
	s.Engine = derefString(engine)
	s.CreatedBy = derefString(createdBy)
	s.UpdatedBy = derefString(updatedBy)
	return s, nil
}

func (r *sessionRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateSessionParams) (domain.Session, error) {
	if !scope.IsValid() {
		return domain.Session{}, coretenant.ErrInvalidScope
	}

	var session domain.Session
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if params.IsDefault {
			if err := unsetDefault(ctx, tx, scope, ""); err != nil {
				return err
			}
		}

		var err error
		session, err = scanSession(tx.QueryRow(ctx, `
			INSERT INTO wa_sessions (
				organization_id, name, display_name, engine, purpose, is_default, auto_create_lead,
				created_by, updated_by
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
			RETURNING `+sessionColumns,
			scope.OrganizationID(), params.Name, nullableString(params.DisplayName), nullableString(params.Engine),
			string(params.Purpose), params.IsDefault, params.AutoCreateLead, nullableString(params.CreatedBy),
		))
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO wa_session_directory (session_name, organization_id, session_id)
			VALUES ($1, $2, $3)
		`, session.Name, scope.OrganizationID(), session.ID)
		return err
	})
	if err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (r *sessionRepository) GetByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Session, error) {
	if !scope.IsValid() {
		return domain.Session{}, coretenant.ErrInvalidScope
	}

	var session domain.Session
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		session, scanErr = scanSession(tx.QueryRow(ctx, `
			SELECT `+sessionColumns+`
			FROM wa_sessions
			WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
		`, id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (r *sessionRepository) List(ctx context.Context, scope coretenant.Scope) ([]domain.Session, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	sessions := []domain.Session{}
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+sessionColumns+`
			FROM wa_sessions
			WHERE organization_id = $1 AND deleted_at IS NULL
			ORDER BY is_default DESC, created_at
		`, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			session, scanErr := scanSession(rows)
			if scanErr != nil {
				return scanErr
			}
			sessions = append(sessions, session)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func (r *sessionRepository) CountActive(ctx context.Context, scope coretenant.Scope) (int64, error) {
	if !scope.IsValid() {
		return 0, coretenant.ErrInvalidScope
	}

	var total int64
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*)
			FROM wa_sessions
			WHERE organization_id = $1 AND deleted_at IS NULL
		`, scope.OrganizationID()).Scan(&total)
	})
	return total, err
}

func (r *sessionRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateSessionParams) (domain.Session, error) {
	if !scope.IsValid() {
		return domain.Session{}, coretenant.ErrInvalidScope
	}

	sets := []string{"updated_at = now()", "updated_by = $3"}
	args := []any{id, scope.OrganizationID(), nullableString(params.UpdatedBy)}
	addSet := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	if params.DisplayName != nil {
		addSet("display_name", nullableString(strings.TrimSpace(*params.DisplayName)))
	}
	if params.IsDefault != nil {
		addSet("is_default", *params.IsDefault)
	}
	if params.Purpose != nil {
		addSet("purpose", string(*params.Purpose))
	}
	if params.AutoCreateLead != nil {
		addSet("auto_create_lead", *params.AutoCreateLead)
	}

	var session domain.Session
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if params.IsDefault != nil && *params.IsDefault {
			if err := unsetDefault(ctx, tx, scope, id); err != nil {
				return err
			}
		}

		var scanErr error
		session, scanErr = scanSession(tx.QueryRow(ctx, `
			UPDATE wa_sessions
			SET `+strings.Join(sets, ", ")+`
			WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
			RETURNING `+sessionColumns,
			args...,
		))
		return scanErr
	})
	if err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (r *sessionRepository) UpdateStatus(ctx context.Context, scope coretenant.Scope, id string, params UpdateSessionStatusParams) (domain.Session, error) {
	if !scope.IsValid() {
		return domain.Session{}, coretenant.ErrInvalidScope
	}

	var session domain.Session
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		session, scanErr = scanSession(tx.QueryRow(ctx, `
			UPDATE wa_sessions
			SET status = $3,
				phone = CASE WHEN $4::boolean THEN $5 ELSE phone END,
				push_name = CASE WHEN $6::boolean THEN $7 ELSE push_name END,
				last_status_at = $8,
				updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
			RETURNING `+sessionColumns,
			id, scope.OrganizationID(), string(params.Status),
			params.Phone != nil, nullableStringPtr(params.Phone),
			params.PushName != nil, nullableStringPtr(params.PushName),
			params.At,
		))
		return scanErr
	})
	if err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (r *sessionRepository) SoftDelete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, `
			UPDATE wa_sessions
			SET deleted_at = now(), is_default = false, updated_by = $3, updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
		`, id, scope.OrganizationID(), nullableString(deletedBy))
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}

		_, err = tx.Exec(ctx, `
			UPDATE wa_session_directory
			SET deleted_at = now()
			WHERE session_id = $1 AND organization_id = $2 AND deleted_at IS NULL
		`, id, scope.OrganizationID())
		return err
	})
}

// unsetDefault clears the organization's current default session, except
// keepID, so the partial unique index allows the new default.
func unsetDefault(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, keepID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE wa_sessions
		SET is_default = false, updated_at = now()
		WHERE organization_id = $1 AND is_default AND deleted_at IS NULL
			AND id IS DISTINCT FROM NULLIF($2, '')::uuid
	`, scope.OrganizationID(), keepID)
	return err
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableStringPtr(value *string) any {
	if value == nil {
		return nil
	}
	return nullableString(*value)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
