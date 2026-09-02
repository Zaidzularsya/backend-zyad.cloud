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
			s.id, s.landing_page_id, s.section_key, s.section_type, s.name,
			s.sort_order, s.is_enabled, s.content, s.style, s.created_at, s.updated_at
		FROM landing_page_sections s
		JOIN landing_pages p ON p.organization_id = s.organization_id AND p.id = s.landing_page_id
		LEFT JOIN organizations o ON o.id = p.organization_id
		WHERE s.landing_page_id = $1
			AND s.deleted_at IS NULL
			AND (
				s.organization_id = $2
				OR (p.is_template = true AND o.type = 'platform')
			)
		ORDER BY s.sort_order ASC
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

func (r *sectionRepository) ReplaceAll(ctx context.Context, scope coretenant.Scope, params ReplaceAllParams) ([]domain.LandingSection, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	orgID := scope.OrganizationID()
	pageID := params.LandingPageID

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		// 1. Lock and load the current live section set for this page.
		rows, err := tx.Query(ctx, `
			SELECT id, section_key
			FROM landing_page_sections
			WHERE landing_page_id = $1 AND organization_id = $2 AND deleted_at IS NULL
			FOR UPDATE
		`, pageID, orgID)
		if err != nil {
			return err
		}

		existingByID := make(map[string]string)  // id -> key
		existingByKey := make(map[string]string) // key -> id
		for rows.Next() {
			var id, key string
			if err := rows.Scan(&id, &key); err != nil {
				rows.Close()
				return err
			}
			existingByID[id] = key
			existingByKey[key] = id
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		// 2. Classify each desired item as update (matched an existing row) or insert.
		type updateOp struct {
			id        string
			item      ReplaceSectionItem
			sortOrder int
		}
		type insertOp struct {
			item      ReplaceSectionItem
			sortOrder int
		}

		matched := make(map[string]bool)
		updates := make([]updateOp, 0, len(params.Items))
		inserts := make([]insertOp, 0, len(params.Items))

		for i, item := range params.Items {
			finalSort := (i + 1) * 10

			matchID := ""
			if item.ID != "" {
				if _, ok := existingByID[item.ID]; ok && !matched[item.ID] {
					matchID = item.ID
				}
			}
			if matchID == "" && item.Key != "" {
				if id, ok := existingByKey[item.Key]; ok && !matched[id] {
					matchID = id
				}
			}

			if matchID != "" {
				matched[matchID] = true
				updates = append(updates, updateOp{id: matchID, item: item, sortOrder: finalSort})
			} else {
				inserts = append(inserts, insertOp{item: item, sortOrder: finalSort})
			}
		}

		// 3. Soft-delete existing rows that are no longer present.
		var deleteIDs []string
		for id := range existingByID {
			if !matched[id] {
				deleteIDs = append(deleteIDs, id)
			}
		}
		if len(deleteIDs) > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE landing_page_sections
				SET deleted_at = NOW()
				WHERE id = ANY($1::uuid[]) AND organization_id = $2 AND deleted_at IS NULL
			`, deleteIDs, orgID); err != nil {
				return err
			}
		}

		// 4. Park every surviving (updated) row in scratch sort_order space so
		//    the partial unique index on (org, page, sort_order) can't trip
		//    while we shuffle. Same technique as Reorder.
		for _, u := range updates {
			if _, err := tx.Exec(ctx, `
				UPDATE landing_page_sections
				SET sort_order = $1 + 1000000
				WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
			`, u.sortOrder, u.id, orgID); err != nil {
				return err
			}
		}

		// 5. Insert new rows directly at their final sort_order (all final slots
		//    are free now: survivors are in scratch space, removed rows are gone).
		for _, in := range inserts {
			var createdBy interface{} = nil
			if params.ActorID != "" {
				createdBy = params.ActorID
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO landing_page_sections (
					organization_id, landing_page_id, section_key, section_type, name,
					sort_order, is_enabled, content, style, created_by
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
				)
			`,
				orgID, pageID, in.item.Key, in.item.Type, in.item.Name,
				in.sortOrder, in.item.IsEnabled, nonNilObject(in.item.Content), nonNilObject(in.item.Style), createdBy,
			); err != nil {
				return err
			}
		}

		// 6. Move surviving rows from scratch space to their final sort_order and
		//    write their mutable fields.
		for _, u := range updates {
			var updatedBy interface{} = nil
			if params.ActorID != "" {
				updatedBy = params.ActorID
			}
			if _, err := tx.Exec(ctx, `
				UPDATE landing_page_sections
				SET sort_order = $1, name = $2, is_enabled = $3, content = $4, style = $5,
					updated_at = NOW(), updated_by = $6
				WHERE id = $7 AND organization_id = $8 AND deleted_at IS NULL
			`,
				u.sortOrder, u.item.Name, u.item.IsEnabled, nonNilObject(u.item.Content), nonNilObject(u.item.Style), updatedBy, u.id, orgID,
			); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return r.ListByPage(ctx, scope, pageID)
}

// nonNilObject guarantees a non-nil map so the NOT NULL / jsonb_typeof='object'
// column constraints are satisfied.
func nonNilObject(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
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
