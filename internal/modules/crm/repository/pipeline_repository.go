package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
)

type pipelineRepository struct {
	db *database.Pool
}

func NewPipelineRepository(db *database.Pool) PipelineRepository {
	return &pipelineRepository{db: db}
}

func (r *pipelineRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

const pipelineColumns = `
	id, organization_id, name, is_default, archived_at, created_by, updated_by, created_at, updated_at, deleted_at
`

const stageColumns = `
	id, organization_id, pipeline_id, name, position, probability::text, is_won, is_lost, created_at, updated_at, deleted_at
`

func scanPipeline(row pgx.Row) (domain.Pipeline, error) {
	var p domain.Pipeline
	var createdBy, updatedBy *string

	err := row.Scan(
		&p.ID, &p.OrganizationID, &p.Name, &p.IsDefault, &p.ArchivedAt,
		&createdBy, &updatedBy, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)
	if err != nil {
		return domain.Pipeline{}, err
	}
	if createdBy != nil {
		p.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		p.UpdatedBy = *updatedBy
	}
	return p, nil
}

func scanStage(row pgx.Row) (domain.PipelineStage, error) {
	var s domain.PipelineStage
	err := row.Scan(
		&s.ID, &s.OrganizationID, &s.PipelineID, &s.Name, &s.Position,
		&s.Probability, &s.IsWon, &s.IsLost, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
	)
	return s, err
}

func loadStages(ctx context.Context, tx pgx.Tx, organizationID string, pipelineID string) ([]domain.PipelineStage, error) {
	query := `SELECT ` + stageColumns + ` FROM crm_pipeline_stages
		WHERE organization_id = $1 AND pipeline_id = $2 AND deleted_at IS NULL
		ORDER BY position ASC`

	rows, err := tx.Query(ctx, query, organizationID, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stages := make([]domain.PipelineStage, 0)
	for rows.Next() {
		stage, err := scanStage(rows)
		if err != nil {
			return nil, err
		}
		stages = append(stages, stage)
	}
	return stages, rows.Err()
}

func insertStage(ctx context.Context, tx pgx.Tx, organizationID string, pipelineID string, input StageInput) error {
	query := `
		INSERT INTO crm_pipeline_stages (organization_id, pipeline_id, name, position, probability, is_won, is_lost)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := tx.Exec(ctx, query, organizationID, pipelineID, input.Name, input.Position, input.Probability, input.IsWon, input.IsLost)
	return err
}

func (r *pipelineRepository) Create(ctx context.Context, scope coretenant.Scope, params CreatePipelineParams) (domain.Pipeline, error) {
	if !scope.IsValid() {
		return domain.Pipeline{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO crm_pipelines (organization_id, name, is_default, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + pipelineColumns

	var pipeline domain.Pipeline
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		pipeline, scanErr = scanPipeline(tx.QueryRow(ctx, query, scope.OrganizationID(), params.Name, params.IsDefault, nullableString(params.CreatedBy)))
		if scanErr != nil {
			return scanErr
		}
		for _, stage := range params.Stages {
			if err := insertStage(ctx, tx, scope.OrganizationID(), pipeline.ID, stage); err != nil {
				return err
			}
		}
		stages, err := loadStages(ctx, tx, scope.OrganizationID(), pipeline.ID)
		if err != nil {
			return err
		}
		pipeline.Stages = stages
		return nil
	})
	if err != nil {
		return domain.Pipeline{}, err
	}
	return pipeline, nil
}

func (r *pipelineRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Pipeline, error) {
	if !scope.IsValid() {
		return domain.Pipeline{}, coretenant.ErrInvalidScope
	}

	query := `SELECT ` + pipelineColumns + ` FROM crm_pipelines WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	var pipeline domain.Pipeline
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		pipeline, scanErr = scanPipeline(tx.QueryRow(ctx, query, id, scope.OrganizationID()))
		if scanErr != nil {
			return scanErr
		}
		stages, err := loadStages(ctx, tx, scope.OrganizationID(), pipeline.ID)
		if err != nil {
			return err
		}
		pipeline.Stages = stages
		return nil
	})
	if err != nil {
		return domain.Pipeline{}, err
	}
	return pipeline, nil
}

func (r *pipelineRepository) List(ctx context.Context, scope coretenant.Scope, filter PipelineListFilter) ([]domain.Pipeline, int64, error) {
	if !scope.IsValid() {
		return nil, 0, coretenant.ErrInvalidScope
	}

	whereClauses := []string{"organization_id = $1"}
	args := []interface{}{scope.OrganizationID()}

	if !filter.IncludeDeleted {
		whereClauses = append(whereClauses, "deleted_at IS NULL")
	}
	if !filter.IncludeArchived {
		whereClauses = append(whereClauses, "archived_at IS NULL")
	}

	where := ""
	for i, clause := range whereClauses {
		if i > 0 {
			where += " AND "
		}
		where += clause
	}
	countQuery := "SELECT COUNT(*) FROM crm_pipelines WHERE " + where

	var pipelines []domain.Pipeline
	var total int64

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			pipelines = []domain.Pipeline{}
			return nil
		}

		query := "SELECT " + pipelineColumns + " FROM crm_pipelines WHERE " + where + " ORDER BY created_at DESC"
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
		for rows.Next() {
			pipeline, scanErr := scanPipeline(rows)
			if scanErr != nil {
				rows.Close()
				return scanErr
			}
			pipelines = append(pipelines, pipeline)
		}
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			return rowsErr
		}

		for i := range pipelines {
			stages, err := loadStages(ctx, tx, scope.OrganizationID(), pipelines[i].ID)
			if err != nil {
				return err
			}
			pipelines[i].Stages = stages
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return pipelines, total, nil
}

func (r *pipelineRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdatePipelineParams) (domain.Pipeline, error) {
	if !scope.IsValid() {
		return domain.Pipeline{}, coretenant.ErrInvalidScope
	}

	setClauses := []string{"updated_at = NOW()"}
	var args []interface{}
	addSet := func(column string, value interface{}) {
		args = append(args, value)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	if params.Name != nil {
		addSet("name", *params.Name)
	}
	if params.IsDefault != nil {
		addSet("is_default", *params.IsDefault)
	}
	if params.UpdatedBy != "" {
		addSet("updated_by", params.UpdatedBy)
	}

	args = append(args, id, scope.OrganizationID())
	query := "UPDATE crm_pipelines SET "
	for i, clause := range setClauses {
		if i > 0 {
			query += ", "
		}
		query += clause
	}
	query += fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", len(args)-1, len(args)) + pipelineColumns

	var pipeline domain.Pipeline
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		pipeline, scanErr = scanPipeline(tx.QueryRow(ctx, query, args...))
		if scanErr != nil {
			return scanErr
		}
		stages, err := loadStages(ctx, tx, scope.OrganizationID(), pipeline.ID)
		if err != nil {
			return err
		}
		pipeline.Stages = stages
		return nil
	})
	if err != nil {
		return domain.Pipeline{}, err
	}
	return pipeline, nil
}

func (r *pipelineRepository) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_pipelines
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

func (r *pipelineRepository) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_pipelines
		SET deleted_at = NULL, updated_by = $1
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NOT NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, nullableString(restoredBy), id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *pipelineRepository) Archive(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Pipeline, error) {
	if !scope.IsValid() {
		return domain.Pipeline{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_pipelines
		SET archived_at = NOW(), updated_by = $1, updated_at = NOW()
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
		RETURNING ` + pipelineColumns

	var pipeline domain.Pipeline
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		pipeline, scanErr = scanPipeline(tx.QueryRow(ctx, query, nullableString(updatedBy), id, scope.OrganizationID()))
		if scanErr != nil {
			return scanErr
		}
		stages, err := loadStages(ctx, tx, scope.OrganizationID(), pipeline.ID)
		if err != nil {
			return err
		}
		pipeline.Stages = stages
		return nil
	})
	if err != nil {
		return domain.Pipeline{}, err
	}
	return pipeline, nil
}

func (r *pipelineRepository) ReplaceStages(ctx context.Context, scope coretenant.Scope, pipelineID string, stages []StageInput, updatedBy string) (domain.Pipeline, error) {
	if !scope.IsValid() {
		return domain.Pipeline{}, coretenant.ErrInvalidScope
	}

	var pipeline domain.Pipeline
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		pipeline, scanErr = scanPipeline(tx.QueryRow(ctx,
			`SELECT `+pipelineColumns+` FROM crm_pipelines WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`,
			pipelineID, scope.OrganizationID(),
		))
		if scanErr != nil {
			return scanErr
		}

		existingIDs := make(map[string]bool)
		rows, err := tx.Query(ctx,
			`SELECT id FROM crm_pipeline_stages WHERE organization_id = $1 AND pipeline_id = $2 AND deleted_at IS NULL`,
			scope.OrganizationID(), pipelineID,
		)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			existingIDs[id] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		keepIDs := make(map[string]bool)
		for _, stage := range stages {
			if stage.ID != nil {
				keepIDs[*stage.ID] = true
				_, err := tx.Exec(ctx, `
					UPDATE crm_pipeline_stages
					SET name = $1, position = $2, probability = $3, is_won = $4, is_lost = $5, updated_at = NOW()
					WHERE id = $6 AND organization_id = $7 AND pipeline_id = $8 AND deleted_at IS NULL
				`, stage.Name, stage.Position, stage.Probability, stage.IsWon, stage.IsLost, *stage.ID, scope.OrganizationID(), pipelineID)
				if err != nil {
					return err
				}
			} else {
				if err := insertStage(ctx, tx, scope.OrganizationID(), pipelineID, stage); err != nil {
					return err
				}
			}
		}

		for id := range existingIDs {
			if !keepIDs[id] {
				_, err := tx.Exec(ctx, `
					UPDATE crm_pipeline_stages SET deleted_at = NOW() WHERE id = $1 AND organization_id = $2
				`, id, scope.OrganizationID())
				if err != nil {
					return err
				}
			}
		}

		if updatedBy != "" {
			if _, err := tx.Exec(ctx,
				`UPDATE crm_pipelines SET updated_by = $1, updated_at = NOW() WHERE id = $2 AND organization_id = $3`,
				updatedBy, pipelineID, scope.OrganizationID(),
			); err != nil {
				return err
			}
		}

		refreshedStages, err := loadStages(ctx, tx, scope.OrganizationID(), pipelineID)
		if err != nil {
			return err
		}
		pipeline.Stages = refreshedStages
		return nil
	})
	if err != nil {
		return domain.Pipeline{}, err
	}
	return pipeline, nil
}

func (r *pipelineRepository) CountActive(ctx context.Context, scope coretenant.Scope) (int64, error) {
	if !scope.IsValid() {
		return 0, coretenant.ErrInvalidScope
	}

	query := `SELECT COUNT(*) FROM crm_pipelines WHERE organization_id = $1 AND deleted_at IS NULL AND archived_at IS NULL`

	var total int64
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, scope.OrganizationID()).Scan(&total)
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}
