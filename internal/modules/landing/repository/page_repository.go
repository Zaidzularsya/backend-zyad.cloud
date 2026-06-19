package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type pageRepository struct {
	db *database.Pool
}

func NewPageRepository(db *database.Pool) PageRepository {
	return &pageRepository{
		db: db,
	}
}

func NewPlatformPageRepository(db *database.Pool) PlatformPageRepository {
	return &platformPageStore{
		db: db,
	}
}

func (r *pageRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

func (r *pageRepository) Create(ctx context.Context, scope coretenant.Scope, params CreatePageParams) (domain.LandingPage, error) {
	if !scope.IsValid() {
		return domain.LandingPage{}, coretenant.ErrInvalidScope
	}
	if params.SEO == nil {
		params.SEO = make(map[string]any)
	}

	query := `
		INSERT INTO landing_pages (
			organization_id, name, title, slug, page_type, status,
			visibility, seo, locale, timezone, is_homepage, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		) RETURNING
			id, organization_id, name, title, slug, page_type, status,
			visibility, password_hash, seo, locale, timezone, is_homepage,
			published_version, publish_at, unpublish_at, published_at,
			created_by, updated_by, created_at, updated_at, deleted_at
	`

	var page domain.LandingPage
	var passwordHash *string
	var createdBy, updatedBy *string

	var createdByInterface interface{} = nil
	if params.CreatedBy != "" {
		createdByInterface = params.CreatedBy
	}

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.Name,
			params.Title,
			params.Slug,
			params.Type,
			params.Status,
			params.Visibility,
			params.SEO,
			params.Locale,
			params.Timezone,
			params.IsHomepage,
			createdByInterface,
		).Scan(
			&page.ID, &page.OrganizationID, &page.Name, &page.Title, &page.Slug,
			&page.Type, &page.Status, &page.Visibility, &passwordHash, &page.SEO,
			&page.Locale, &page.Timezone, &page.IsHomepage,
			&page.PublishedVersion, &page.PublishAt, &page.UnpublishAt, &page.PublishedAt,
			&createdBy, &updatedBy, &page.CreatedAt, &page.UpdatedAt, &page.DeletedAt,
		)
	})

	if err != nil {
		return domain.LandingPage{}, err
	}

	if passwordHash != nil {
		page.PasswordHash = *passwordHash
	}
	if createdBy != nil {
		page.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		page.UpdatedBy = *updatedBy
	}

	return page, nil
}

func (r *pageRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingPage, error) {
	if !scope.IsValid() {
		return domain.LandingPage{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, organization_id, name, title, slug, page_type, status,
			visibility, password_hash, seo, locale, timezone, is_homepage,
			published_version, publish_at, unpublish_at, published_at,
			created_by, updated_by, created_at, updated_at, deleted_at
		FROM landing_pages
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	var page domain.LandingPage
	var passwordHash *string
	var createdBy, updatedBy *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&page.ID, &page.OrganizationID, &page.Name, &page.Title, &page.Slug,
			&page.Type, &page.Status, &page.Visibility, &passwordHash, &page.SEO,
			&page.Locale, &page.Timezone, &page.IsHomepage,
			&page.PublishedVersion, &page.PublishAt, &page.UnpublishAt, &page.PublishedAt,
			&createdBy, &updatedBy, &page.CreatedAt, &page.UpdatedAt, &page.DeletedAt,
		)
	})

	if err != nil {
		return domain.LandingPage{}, err
	}

	if passwordHash != nil {
		page.PasswordHash = *passwordHash
	}
	if createdBy != nil {
		page.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		page.UpdatedBy = *updatedBy
	}

	return page, nil
}

func (r *pageRepository) List(ctx context.Context, scope coretenant.Scope, filter PageListFilter) ([]domain.LandingPage, int64, error) {
	if !scope.IsValid() {
		return nil, 0, coretenant.ErrInvalidScope
	}

	var count int64
	countQuery := `SELECT COUNT(*) FROM landing_pages WHERE organization_id = $1`
	var countArgs []interface{}
	countArgs = append(countArgs, scope.OrganizationID())

	whereClauses := []string{}
	if !filter.IncludeDeleted {
		whereClauses = append(whereClauses, "deleted_at IS NULL")
	}
	if filter.Status != "" {
		countArgs = append(countArgs, filter.Status)
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", len(countArgs)))
	}

	if len(whereClauses) > 0 {
		countQuery += " AND " + strings.Join(whereClauses, " AND ")
	}

	var pages []domain.LandingPage

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, countQuery, countArgs...).Scan(&count)
		if err != nil {
			return err
		}

		if count == 0 {
			pages = []domain.LandingPage{}
			return nil
		}

		query := `
			SELECT
				id, organization_id, name, title, slug, page_type, status,
				visibility, password_hash, seo, locale, timezone, is_homepage,
				published_version, publish_at, unpublish_at, published_at,
				created_by, updated_by, created_at, updated_at, deleted_at
			FROM landing_pages
			WHERE organization_id = $1
		`
		if len(whereClauses) > 0 {
			query += " AND " + strings.Join(whereClauses, " AND ")
		}
		query += " ORDER BY created_at DESC"

		var queryArgs []interface{}
		queryArgs = append(queryArgs, countArgs...)

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
			var page domain.LandingPage
			var passwordHash *string
			var createdBy, updatedBy *string

			err := rows.Scan(
				&page.ID, &page.OrganizationID, &page.Name, &page.Title, &page.Slug,
				&page.Type, &page.Status, &page.Visibility, &passwordHash, &page.SEO,
				&page.Locale, &page.Timezone, &page.IsHomepage,
				&page.PublishedVersion, &page.PublishAt, &page.UnpublishAt, &page.PublishedAt,
				&createdBy, &updatedBy, &page.CreatedAt, &page.UpdatedAt, &page.DeletedAt,
			)
			if err != nil {
				return err
			}

			if passwordHash != nil {
				page.PasswordHash = *passwordHash
			}
			if createdBy != nil {
				page.CreatedBy = *createdBy
			}
			if updatedBy != nil {
				page.UpdatedBy = *updatedBy
			}

			pages = append(pages, page)
		}

		return rows.Err()
	})

	if err != nil {
		return nil, 0, err
	}

	return pages, count, nil
}

func (r *pageRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdatePageParams) (domain.LandingPage, error) {
	if !scope.IsValid() {
		return domain.LandingPage{}, coretenant.ErrInvalidScope
	}

	// Dynamic update
	query := `UPDATE landing_pages SET updated_at = NOW()`
	var args []interface{}
	argCount := 1

	if params.Name != nil {
		args = append(args, *params.Name)
		query += fmt.Sprintf(", name = $%d", argCount)
		argCount++
	}
	if params.Title != nil {
		args = append(args, *params.Title)
		query += fmt.Sprintf(", title = $%d", argCount)
		argCount++
	}
	if params.Slug != nil {
		args = append(args, *params.Slug)
		query += fmt.Sprintf(", slug = $%d", argCount)
		argCount++
	}
	if params.Type != nil {
		args = append(args, *params.Type)
		query += fmt.Sprintf(", page_type = $%d", argCount)
		argCount++
	}
	if params.Status != nil {
		args = append(args, *params.Status)
		query += fmt.Sprintf(", status = $%d", argCount)
		argCount++
	}
	if params.Visibility != nil {
		args = append(args, *params.Visibility)
		query += fmt.Sprintf(", visibility = $%d", argCount)
		argCount++
	}
	if params.PasswordHash != nil {
		if *params.PasswordHash == "" {
			query += ", password_hash = NULL"
		} else {
			args = append(args, *params.PasswordHash)
			query += fmt.Sprintf(", password_hash = $%d", argCount)
			argCount++
		}
	}
	if params.SEO != nil {
		args = append(args, params.SEO)
		query += fmt.Sprintf(", seo = $%d", argCount)
		argCount++
	}
	if params.Locale != nil {
		args = append(args, *params.Locale)
		query += fmt.Sprintf(", locale = $%d", argCount)
		argCount++
	}
	if params.Timezone != nil {
		args = append(args, *params.Timezone)
		query += fmt.Sprintf(", timezone = $%d", argCount)
		argCount++
	}
	if params.IsHomepage != nil {
		args = append(args, *params.IsHomepage)
		query += fmt.Sprintf(", is_homepage = $%d", argCount)
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
		id, organization_id, name, title, slug, page_type, status,
		visibility, password_hash, seo, locale, timezone, is_homepage,
		published_version, publish_at, unpublish_at, published_at,
		created_by, updated_by, created_at, updated_at, deleted_at
	`

	var page domain.LandingPage
	var passwordHash *string
	var createdBy, updatedBy *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(
			&page.ID, &page.OrganizationID, &page.Name, &page.Title, &page.Slug,
			&page.Type, &page.Status, &page.Visibility, &passwordHash, &page.SEO,
			&page.Locale, &page.Timezone, &page.IsHomepage,
			&page.PublishedVersion, &page.PublishAt, &page.UnpublishAt, &page.PublishedAt,
			&createdBy, &updatedBy, &page.CreatedAt, &page.UpdatedAt, &page.DeletedAt,
		)
	})

	if err != nil {
		return domain.LandingPage{}, err
	}

	if passwordHash != nil {
		page.PasswordHash = *passwordHash
	}
	if createdBy != nil {
		page.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		page.UpdatedBy = *updatedBy
	}

	return page, nil
}

func (r *pageRepository) Delete(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	// Soft delete
	query := `
		UPDATE landing_pages
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

type platformPageStore struct {
	db *database.Pool
}

func (r *platformPageStore) FindByOrganizationAndID(ctx context.Context, organizationID string, id string) (domain.LandingPage, error) {
	query := `
		SELECT
			id, organization_id, name, title, slug, page_type, status,
			visibility, password_hash, seo, locale, timezone, is_homepage,
			published_version, publish_at, unpublish_at, published_at,
			created_by, updated_by, created_at, updated_at, deleted_at
		FROM landing_pages
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	var page domain.LandingPage
	var passwordHash *string
	var createdBy, updatedBy *string

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.LandingPage{}, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", organizationID)
	if err != nil {
		return domain.LandingPage{}, err
	}

	err = tx.QueryRow(ctx, query, id, organizationID).Scan(
		&page.ID, &page.OrganizationID, &page.Name, &page.Title, &page.Slug,
		&page.Type, &page.Status, &page.Visibility, &passwordHash, &page.SEO,
		&page.Locale, &page.Timezone, &page.IsHomepage,
		&page.PublishedVersion, &page.PublishAt, &page.UnpublishAt, &page.PublishedAt,
		&createdBy, &updatedBy, &page.CreatedAt, &page.UpdatedAt, &page.DeletedAt,
	)

	if err != nil {
		return domain.LandingPage{}, err
	}

	if passwordHash != nil {
		page.PasswordHash = *passwordHash
	}
	if createdBy != nil {
		page.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		page.UpdatedBy = *updatedBy
	}

	return page, nil
}

func (r *platformPageStore) ListByOrganization(ctx context.Context, organizationID string, filter PageListFilter) ([]domain.LandingPage, int64, error) {
	// Not fully implemented yet
	return nil, 0, nil
}
