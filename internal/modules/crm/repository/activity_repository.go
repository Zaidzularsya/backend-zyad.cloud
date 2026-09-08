package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
)

type activityRepository struct {
	db *database.Pool
}

func NewActivityRepository(db *database.Pool) ActivityRepository {
	return &activityRepository{db: db}
}

func (r *activityRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

const activityColumns = `
	id, organization_id, related_entity_type, related_entity_id, type, subject, description,
	due_at, completed_at, status, assignee_user_id, created_by, updated_by,
	created_at, updated_at, deleted_at
`

func scanActivity(row pgx.Row) (domain.Activity, error) {
	var a domain.Activity
	var description *string
	var assigneeUserID, createdBy, updatedBy *string
	var relatedEntityType, activityType, status string

	err := row.Scan(
		&a.ID, &a.OrganizationID, &relatedEntityType, &a.RelatedEntityID, &activityType, &a.Subject, &description,
		&a.DueAt, &a.CompletedAt, &status, &assigneeUserID, &createdBy, &updatedBy,
		&a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
	)
	if err != nil {
		return domain.Activity{}, err
	}

	a.RelatedEntityType = domain.ActivityEntityType(relatedEntityType)
	a.Type = domain.ActivityType(activityType)
	a.Status = domain.ActivityStatus(status)
	if description != nil {
		a.Description = *description
	}
	if assigneeUserID != nil {
		a.AssigneeUserID = *assigneeUserID
	}
	if createdBy != nil {
		a.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		a.UpdatedBy = *updatedBy
	}

	return a, nil
}

func (r *activityRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateActivityParams) (domain.Activity, error) {
	if !scope.IsValid() {
		return domain.Activity{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO crm_activities (
			organization_id, related_entity_type, related_entity_id, type, subject, description,
			due_at, assignee_user_id, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		) RETURNING ` + activityColumns

	var activity domain.Activity
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		activity, scanErr = scanActivity(tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			string(params.RelatedEntityType),
			params.RelatedEntityID,
			string(params.Type),
			params.Subject,
			nullableString(params.Description),
			params.DueAt,
			nullableString(params.AssigneeUserID),
			nullableString(params.CreatedBy),
		))
		return scanErr
	})
	if err != nil {
		return domain.Activity{}, err
	}
	return activity, nil
}

func (r *activityRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Activity, error) {
	if !scope.IsValid() {
		return domain.Activity{}, coretenant.ErrInvalidScope
	}

	query := `SELECT ` + activityColumns + ` FROM crm_activities WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	var activity domain.Activity
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		activity, scanErr = scanActivity(tx.QueryRow(ctx, query, id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Activity{}, err
	}
	return activity, nil
}

func (r *activityRepository) List(ctx context.Context, scope coretenant.Scope, filter ActivityListFilter) ([]domain.Activity, int64, error) {
	if !scope.IsValid() {
		return nil, 0, coretenant.ErrInvalidScope
	}

	whereClauses := []string{"organization_id = $1"}
	args := []interface{}{scope.OrganizationID()}

	if !filter.IncludeDeleted {
		whereClauses = append(whereClauses, "deleted_at IS NULL")
	}
	if filter.RelatedEntityType != "" {
		args = append(args, string(filter.RelatedEntityType))
		whereClauses = append(whereClauses, fmt.Sprintf("related_entity_type = $%d", len(args)))
	}
	if filter.RelatedEntityID != "" {
		args = append(args, filter.RelatedEntityID)
		whereClauses = append(whereClauses, fmt.Sprintf("related_entity_id = $%d", len(args)))
	}
	if filter.AssigneeUserID != "" {
		args = append(args, filter.AssigneeUserID)
		whereClauses = append(whereClauses, fmt.Sprintf("assignee_user_id = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", len(args)))
	}

	where := strings.Join(whereClauses, " AND ")
	countQuery := "SELECT COUNT(*) FROM crm_activities WHERE " + where

	var activities []domain.Activity
	var total int64

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			activities = []domain.Activity{}
			return nil
		}

		query := "SELECT " + activityColumns + " FROM crm_activities WHERE " + where + " ORDER BY due_at NULLS LAST, created_at DESC"
		queryArgs := append([]interface{}{}, args...)
		if filter.Limit > 0 {
			queryArgs = append(queryArgs, filter.Limit)
			query += fmt.Sprintf(" LIMIT $%d", len(queryArgs))
		}
		if filter.Offset > 0 {
			queryArgs = append(queryArgs, filter.Offset)
			query += fmt.Sprintf(" OFFSET $%d", len(queryArgs))
		}

		rows, err := tx.Query(ctx, query, queryArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			activity, scanErr := scanActivity(rows)
			if scanErr != nil {
				return scanErr
			}
			activities = append(activities, activity)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}

	return activities, total, nil
}

func (r *activityRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateActivityParams) (domain.Activity, error) {
	if !scope.IsValid() {
		return domain.Activity{}, coretenant.ErrInvalidScope
	}

	setClauses := []string{"updated_at = NOW()"}
	var args []interface{}
	addSet := func(column string, value interface{}) {
		args = append(args, value)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if params.Type != nil {
		addSet("type", string(*params.Type))
	}
	if params.Subject != nil {
		addSet("subject", *params.Subject)
	}
	if params.Description != nil {
		addSet("description", *params.Description)
	}
	if params.DueAt != nil {
		addSet("due_at", *params.DueAt)
	}
	if params.AssigneeUserID != nil {
		addSet("assignee_user_id", nullableString(*params.AssigneeUserID))
	}
	if params.UpdatedBy != "" {
		addSet("updated_by", params.UpdatedBy)
	}

	args = append(args, id, scope.OrganizationID())
	query := "UPDATE crm_activities SET " + strings.Join(setClauses, ", ") +
		fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", len(args)-1, len(args)) +
		activityColumns

	var activity domain.Activity
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		activity, scanErr = scanActivity(tx.QueryRow(ctx, query, args...))
		return scanErr
	})
	if err != nil {
		return domain.Activity{}, err
	}
	return activity, nil
}

func (r *activityRepository) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_activities
		SET deleted_at = NOW(), updated_by = $1
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, nullableString(deletedBy), id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *activityRepository) Complete(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Activity, error) {
	if !scope.IsValid() {
		return domain.Activity{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_activities
		SET status = 'completed', completed_at = NOW(), updated_by = $1, updated_at = NOW()
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL AND status = 'pending'
		RETURNING ` + activityColumns

	var activity domain.Activity
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		activity, scanErr = scanActivity(tx.QueryRow(ctx, query, nullableString(updatedBy), id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Activity{}, err
	}
	return activity, nil
}

func (r *activityRepository) Cancel(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Activity, error) {
	if !scope.IsValid() {
		return domain.Activity{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_activities
		SET status = 'cancelled', updated_by = $1, updated_at = NOW()
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL AND status = 'pending'
		RETURNING ` + activityColumns

	var activity domain.Activity
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		activity, scanErr = scanActivity(tx.QueryRow(ctx, query, nullableString(updatedBy), id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Activity{}, err
	}
	return activity, nil
}

func (r *activityRepository) Assign(ctx context.Context, scope coretenant.Scope, id string, assigneeUserID string, updatedBy string) (domain.Activity, error) {
	if !scope.IsValid() {
		return domain.Activity{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_activities
		SET assignee_user_id = $1, updated_by = $2, updated_at = NOW()
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL
		RETURNING ` + activityColumns

	var activity domain.Activity
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		activity, scanErr = scanActivity(tx.QueryRow(ctx, query, assigneeUserID, nullableString(updatedBy), id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Activity{}, err
	}
	return activity, nil
}
