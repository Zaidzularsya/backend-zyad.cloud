package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type mediaRepository struct {
	db *database.Pool
}

func NewMediaRepository(db *database.Pool) MediaRepository {
	return &mediaRepository{db: db}
}

func (r *mediaRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

func (r *mediaRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateMediaAssetParams) (domain.LandingMediaAsset, error) {
	if !scope.IsValid() {
		return domain.LandingMediaAsset{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_media_assets (
			organization_id, storage_key, filename, mime_type, size_bytes,
			width, height, duration_seconds, alt_text, processing_status, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		) RETURNING
			id, storage_key, filename, mime_type, size_bytes,
			width, height, duration_seconds, alt_text, processing_status, created_by,
			created_at, updated_at
	`

	var m domain.LandingMediaAsset
	var createdBy, altText *string

	if params.AltText != "" {
		altText = &params.AltText
	}
	var cb interface{} = nil
	if params.CreatedBy != "" {
		cb = params.CreatedBy
	}

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.StorageKey,
			params.Filename,
			params.MimeType,
			params.SizeBytes,
			params.Width,
			params.Height,
			params.DurationSeconds,
			altText,
			params.ProcessingStatus,
			cb,
		).Scan(
			&m.ID, &m.StorageKey, &m.Filename, &m.MimeType, &m.SizeBytes,
			&m.Width, &m.Height, &m.DurationSeconds, &altText, &m.ProcessingStatus, &createdBy,
			&m.CreatedAt, &m.UpdatedAt,
		)
		return err
	})

	if err != nil {
		return domain.LandingMediaAsset{}, err
	}
	m.OrganizationID = scope.OrganizationID()
	if createdBy != nil { m.CreatedBy = *createdBy }
	if altText != nil { m.AltText = *altText }

	return m, nil
}

func (r *mediaRepository) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingMediaAsset, error) {
	if !scope.IsValid() {
		return domain.LandingMediaAsset{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, storage_key, filename, mime_type, size_bytes,
			width, height, duration_seconds, alt_text, processing_status, created_by,
			created_at, updated_at
		FROM landing_media_assets
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	var m domain.LandingMediaAsset
	var createdBy, altText *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&m.ID, &m.StorageKey, &m.Filename, &m.MimeType, &m.SizeBytes,
			&m.Width, &m.Height, &m.DurationSeconds, &altText, &m.ProcessingStatus, &createdBy,
			&m.CreatedAt, &m.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingMediaAsset{}, err
	}
	m.OrganizationID = scope.OrganizationID()
	if createdBy != nil { m.CreatedBy = *createdBy }
	if altText != nil { m.AltText = *altText }

	return m, nil
}

func (r *mediaRepository) List(ctx context.Context, scope coretenant.Scope) ([]domain.LandingMediaAsset, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, storage_key, filename, mime_type, size_bytes,
			width, height, duration_seconds, alt_text, processing_status, created_by,
			created_at, updated_at
		FROM landing_media_assets
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	var assets []domain.LandingMediaAsset

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m domain.LandingMediaAsset
			var createdBy, altText *string

			err := rows.Scan(
				&m.ID, &m.StorageKey, &m.Filename, &m.MimeType, &m.SizeBytes,
				&m.Width, &m.Height, &m.DurationSeconds, &altText, &m.ProcessingStatus, &createdBy,
				&m.CreatedAt, &m.UpdatedAt,
			)
			if err != nil {
				return err
			}
			m.OrganizationID = scope.OrganizationID()
			if createdBy != nil { m.CreatedBy = *createdBy }
			if altText != nil { m.AltText = *altText }
			assets = append(assets, m)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return assets, nil
}

func (r *mediaRepository) UpdateStatus(ctx context.Context, scope coretenant.Scope, id string, status domain.MediaProcessingStatus) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE landing_media_assets
		SET processing_status = $1, updated_at = NOW()
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, status, id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *mediaRepository) Delete(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	// Soft delete
	query := `
		UPDATE landing_media_assets
		SET deleted_at = NOW()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
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
