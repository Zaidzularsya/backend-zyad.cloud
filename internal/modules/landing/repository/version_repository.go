package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type versionRepository struct {
	db *database.Pool
}

func NewVersionRepository(db *database.Pool) VersionRepository {
	return &versionRepository{db: db}
}

func (r *versionRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

func (r *versionRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateVersionParams) (domain.LandingPageVersion, error) {
	if !scope.IsValid() {
		return domain.LandingPageVersion{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_page_versions (
			organization_id, landing_page_id, version, change_note, snapshot, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6
		) RETURNING
			id, landing_page_id, version, change_note, snapshot, created_by, created_at
	`

	var ver domain.LandingPageVersion
	var createdBy interface{} = nil
	if params.CreatedBy != "" {
		createdBy = params.CreatedBy
	}

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var createdByResult *string
		err := tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.LandingPageID,
			params.Version,
			params.ChangeNote,
			params.Snapshot,
			createdBy,
		).Scan(
			&ver.ID, &ver.LandingPageID, &ver.Version, &ver.ChangeNote,
			&ver.Snapshot, &createdByResult, &ver.CreatedAt,
		)
		if err != nil {
			return err
		}
		if createdByResult != nil {
			ver.CreatedBy = *createdByResult
		}
		return nil
	})

	if err != nil {
		return domain.LandingPageVersion{}, err
	}
	ver.OrganizationID = scope.OrganizationID()

	return ver, nil
}

func (r *versionRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingPageVersion, error) {
	if !scope.IsValid() {
		return domain.LandingPageVersion{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, version, change_note, snapshot, created_by, created_at
		FROM landing_page_versions
		WHERE id = $1 AND organization_id = $2
	`

	var ver domain.LandingPageVersion

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var createdByResult *string
		err := tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&ver.ID, &ver.LandingPageID, &ver.Version, &ver.ChangeNote,
			&ver.Snapshot, &createdByResult, &ver.CreatedAt,
		)
		if err != nil {
			return err
		}
		if createdByResult != nil {
			ver.CreatedBy = *createdByResult
		}
		return nil
	})

	if err != nil {
		return domain.LandingPageVersion{}, err
	}
	ver.OrganizationID = scope.OrganizationID()

	return ver, nil
}

func (r *versionRepository) FindByVersion(ctx context.Context, scope coretenant.Scope, pageID string, version int) (domain.LandingPageVersion, error) {
	if !scope.IsValid() {
		return domain.LandingPageVersion{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, version, change_note, snapshot, created_by, created_at
		FROM landing_page_versions
		WHERE landing_page_id = $1 AND version = $2 AND organization_id = $3
	`

	var ver domain.LandingPageVersion

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var createdByResult *string
		err := tx.QueryRow(ctx, query, pageID, version, scope.OrganizationID()).Scan(
			&ver.ID, &ver.LandingPageID, &ver.Version, &ver.ChangeNote,
			&ver.Snapshot, &createdByResult, &ver.CreatedAt,
		)
		if err != nil {
			return err
		}
		if createdByResult != nil {
			ver.CreatedBy = *createdByResult
		}
		return nil
	})

	if err != nil {
		return domain.LandingPageVersion{}, err
	}
	ver.OrganizationID = scope.OrganizationID()

	return ver, nil
}

func (r *versionRepository) ListByPage(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingPageVersion, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, version, change_note, snapshot, created_by, created_at
		FROM landing_page_versions
		WHERE landing_page_id = $1 AND organization_id = $2
		ORDER BY version DESC
	`

	var versions []domain.LandingPageVersion

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, pageID, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var ver domain.LandingPageVersion
			var createdByResult *string
			err := rows.Scan(
				&ver.ID, &ver.LandingPageID, &ver.Version, &ver.ChangeNote,
				&ver.Snapshot, &createdByResult, &ver.CreatedAt,
			)
			if err != nil {
				return err
			}
			if createdByResult != nil {
				ver.CreatedBy = *createdByResult
			}
			ver.OrganizationID = scope.OrganizationID()
			versions = append(versions, ver)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return versions, nil
}

func (r *versionRepository) CreateRedirect(ctx context.Context, scope coretenant.Scope, params CreateSlugRedirectParams) (domain.LandingSlugRedirect, error) {
	if !scope.IsValid() {
		return domain.LandingSlugRedirect{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_slug_redirects (
			organization_id, source_slug, target_slug
		) VALUES (
			$1, $2, $3
		) RETURNING
			id, source_slug, target_slug, created_at, updated_at
	`

	var redirect domain.LandingSlugRedirect

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.SourceSlug,
			params.TargetSlug,
		).Scan(
			&redirect.ID, &redirect.SourceSlug, &redirect.TargetSlug,
			&redirect.CreatedAt, &redirect.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingSlugRedirect{}, err
	}
	redirect.OrganizationID = scope.OrganizationID()

	return redirect, nil
}

func (r *versionRepository) FindBySourceSlug(ctx context.Context, scope coretenant.Scope, sourceSlug string) (domain.LandingSlugRedirect, error) {
	if !scope.IsValid() {
		return domain.LandingSlugRedirect{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, source_slug, target_slug, created_at, updated_at
		FROM landing_slug_redirects
		WHERE lower(source_slug) = lower($1) AND organization_id = $2
	`

	var redirect domain.LandingSlugRedirect

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, sourceSlug, scope.OrganizationID()).Scan(
			&redirect.ID, &redirect.SourceSlug, &redirect.TargetSlug,
			&redirect.CreatedAt, &redirect.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingSlugRedirect{}, err
	}
	redirect.OrganizationID = scope.OrganizationID()

	return redirect, nil
}

func (r *versionRepository) ListRedirects(ctx context.Context, scope coretenant.Scope) ([]domain.LandingSlugRedirect, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, source_slug, target_slug, created_at, updated_at
		FROM landing_slug_redirects
		WHERE organization_id = $1
		ORDER BY created_at DESC
	`

	var redirects []domain.LandingSlugRedirect

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var redirect domain.LandingSlugRedirect
			err := rows.Scan(
				&redirect.ID, &redirect.SourceSlug, &redirect.TargetSlug,
				&redirect.CreatedAt, &redirect.UpdatedAt,
			)
			if err != nil {
				return err
			}
			redirect.OrganizationID = scope.OrganizationID()
			redirects = append(redirects, redirect)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return redirects, nil
}

func (r *versionRepository) UpdateRedirect(ctx context.Context, scope coretenant.Scope, id string, targetSlug string) (domain.LandingSlugRedirect, error) {
	if !scope.IsValid() {
		return domain.LandingSlugRedirect{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE landing_slug_redirects
		SET target_slug = $1, updated_at = NOW()
		WHERE id = $2 AND organization_id = $3
		RETURNING id, source_slug, target_slug, created_at, updated_at
	`

	var redirect domain.LandingSlugRedirect

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, targetSlug, id, scope.OrganizationID()).Scan(
			&redirect.ID, &redirect.SourceSlug, &redirect.TargetSlug,
			&redirect.CreatedAt, &redirect.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingSlugRedirect{}, err
	}
	redirect.OrganizationID = scope.OrganizationID()

	return redirect, nil
}

func (r *versionRepository) DeleteRedirect(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		DELETE FROM landing_slug_redirects
		WHERE id = $1 AND organization_id = $2
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
