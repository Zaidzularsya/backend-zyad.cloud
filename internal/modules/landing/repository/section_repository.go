package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type sectionRepository struct {
	db *database.Pool
}

func NewSectionRepository(db *database.Pool) SectionRepository {
	return &sectionRepository{
		db: db,
	}
}

func (r *sectionRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

func (r *sectionRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateSectionParams) (domain.LandingSection, error) {
	if !scope.IsValid() {
		return domain.LandingSection{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_page_sections (
			organization_id, landing_page_id, section_key, section_type, name,
			sort_order, is_enabled, content, style, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		) RETURNING
			id, landing_page_id, section_key, section_type, name,
			sort_order, is_enabled, content, style, created_at, updated_at
	`

	var section domain.LandingSection

	var createdBy interface{} = nil
	if params.CreatedBy != "" {
		createdBy = params.CreatedBy
	}

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.LandingPageID,
			params.Key,
			params.Type,
			params.Name,
			params.SortOrder,
			params.IsEnabled,
			params.Content,
			params.Style,
			createdBy,
		).Scan(
			&section.ID, &section.LandingPageID, &section.Key, &section.Type, &section.Name,
			&section.SortOrder, &section.IsEnabled, &section.Content, &section.Style,
			&section.CreatedAt, &section.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingSection{}, err
	}

	return section, nil
}

func (r *sectionRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingSection, error) {
	if !scope.IsValid() {
		return domain.LandingSection{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, section_key, section_type, name,
			sort_order, is_enabled, content, style, created_at, updated_at
		FROM landing_page_sections
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	var section domain.LandingSection

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&section.ID, &section.LandingPageID, &section.Key, &section.Type, &section.Name,
			&section.SortOrder, &section.IsEnabled, &section.Content, &section.Style,
			&section.CreatedAt, &section.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingSection{}, err
	}

	return section, nil
}

func (r *sectionRepository) ListByPage(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingSection, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, section_key, section_type, name,
			sort_order, is_enabled, content, style, created_at, updated_at
		FROM landing_page_sections
		WHERE landing_page_id = $1 AND organization_id = $2 AND deleted_at IS NULL
		ORDER BY sort_order ASC
	`

	var sections []domain.LandingSection

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, pageID, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var section domain.LandingSection
			err := rows.Scan(
				&section.ID, &section.LandingPageID, &section.Key, &section.Type, &section.Name,
				&section.SortOrder, &section.IsEnabled, &section.Content, &section.Style,
				&section.CreatedAt, &section.UpdatedAt,
			)
			if err != nil {
				return err
			}
			sections = append(sections, section)
		}

		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return sections, nil
}

func (r *sectionRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateSectionParams) (domain.LandingSection, error) {
	if !scope.IsValid() {
		return domain.LandingSection{}, coretenant.ErrInvalidScope
	}

	query := `UPDATE landing_page_sections SET updated_at = NOW()`
	var args []interface{}
	argCount := 1

	if params.Name != nil {
		args = append(args, *params.Name)
		query += fmt.Sprintf(", name = $%d", argCount)
		argCount++
	}
	if params.IsEnabled != nil {
		args = append(args, *params.IsEnabled)
		query += fmt.Sprintf(", is_enabled = $%d", argCount)
		argCount++
	}
	if params.Content != nil {
		args = append(args, params.Content)
		query += fmt.Sprintf(", content = $%d", argCount)
		argCount++
	}
	if params.Style != nil {
		args = append(args, params.Style)
		query += fmt.Sprintf(", style = $%d", argCount)
		argCount++
	}
	if params.UpdatedBy != "" {
		args = append(args, params.UpdatedBy)
		query += fmt.Sprintf(", updated_by = $%d", argCount)
		argCount++
	}

	args = append(args, id, scope.OrganizationID())
	query += fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", argCount, argCount+1)
	query += `
		id, landing_page_id, section_key, section_type, name,
		sort_order, is_enabled, content, style, created_at, updated_at
	`

	var section domain.LandingSection

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(
			&section.ID, &section.LandingPageID, &section.Key, &section.Type, &section.Name,
			&section.SortOrder, &section.IsEnabled, &section.Content, &section.Style,
			&section.CreatedAt, &section.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingSection{}, err
	}

	return section, nil
}

func (r *sectionRepository) Reorder(ctx context.Context, scope coretenant.Scope, pageID string, params []SectionReorderParam) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		for _, p := range params {
			query1 := `
				UPDATE landing_page_sections 
				SET sort_order = $1 + 1000000 
				WHERE id = $2 AND landing_page_id = $3 AND organization_id = $4 AND deleted_at IS NULL
			`
			_, err := tx.Exec(ctx, query1, p.SortOrder, p.ID, pageID, scope.OrganizationID())
			if err != nil {
				return err
			}
		}

		for _, p := range params {
			query2 := `
				UPDATE landing_page_sections 
				SET sort_order = $1 
				WHERE id = $2 AND landing_page_id = $3 AND organization_id = $4 AND deleted_at IS NULL
			`
			_, err := tx.Exec(ctx, query2, p.SortOrder, p.ID, pageID, scope.OrganizationID())
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *sectionRepository) Delete(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE landing_page_sections
		SET deleted_at = NOW()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})

	return err
}
