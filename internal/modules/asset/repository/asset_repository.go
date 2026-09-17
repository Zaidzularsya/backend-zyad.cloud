package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/asset/domain"
	"zyad.cloud/internal/platform/database"
)

type assetRepository struct {
	db *database.Pool
}

func NewAssetRepository(db *database.Pool) AssetRepository {
	return &assetRepository{db: db}
}

func (r *assetRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

func (r *assetRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateAssetObjectParams) (domain.AssetObject, error) {
	if !scope.IsValid() {
		return domain.AssetObject{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO asset_objects (
			organization_id, storage_key, filename, mime_type, size_bytes, class, label, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		) RETURNING
			id, storage_key, filename, mime_type, size_bytes, class, label, created_by,
			created_at, updated_at
	`

	var a domain.AssetObject
	var createdBy, label *string

	if params.Label != "" {
		label = &params.Label
	}
	var cb interface{} = nil
	if params.CreatedBy != "" {
		cb = params.CreatedBy
	}

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.StorageKey,
			params.Filename,
			params.MimeType,
			params.SizeBytes,
			params.Class,
			label,
			cb,
		).Scan(
			&a.ID, &a.StorageKey, &a.Filename, &a.MimeType, &a.SizeBytes, &a.Class, &label, &createdBy,
			&a.CreatedAt, &a.UpdatedAt,
		)
	})

	if err != nil {
		return domain.AssetObject{}, err
	}
	a.OrganizationID = scope.OrganizationID()
	if createdBy != nil {
		a.CreatedBy = *createdBy
	}
	if label != nil {
		a.Label = *label
	}

	return a, nil
}

func (r *assetRepository) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.AssetObject, error) {
	if !scope.IsValid() {
		return domain.AssetObject{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, storage_key, filename, mime_type, size_bytes, class, label, created_by,
			created_at, updated_at
		FROM asset_objects
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	var a domain.AssetObject
	var createdBy, label *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&a.ID, &a.StorageKey, &a.Filename, &a.MimeType, &a.SizeBytes, &a.Class, &label, &createdBy,
			&a.CreatedAt, &a.UpdatedAt,
		)
	})

	if err != nil {
		return domain.AssetObject{}, err
	}
	a.OrganizationID = scope.OrganizationID()
	if createdBy != nil {
		a.CreatedBy = *createdBy
	}
	if label != nil {
		a.Label = *label
	}

	return a, nil
}

func (r *assetRepository) List(ctx context.Context, scope coretenant.Scope) ([]domain.AssetObject, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, storage_key, filename, mime_type, size_bytes, class, label, created_by,
			created_at, updated_at
		FROM asset_objects
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	var objects []domain.AssetObject

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var a domain.AssetObject
			var createdBy, label *string

			if err := rows.Scan(
				&a.ID, &a.StorageKey, &a.Filename, &a.MimeType, &a.SizeBytes, &a.Class, &label, &createdBy,
				&a.CreatedAt, &a.UpdatedAt,
			); err != nil {
				return err
			}
			a.OrganizationID = scope.OrganizationID()
			if createdBy != nil {
				a.CreatedBy = *createdBy
			}
			if label != nil {
				a.Label = *label
			}
			objects = append(objects, a)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return objects, nil
}

func (r *assetRepository) Delete(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE asset_objects
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

func (r *assetRepository) SumSizeBytes(ctx context.Context, scope coretenant.Scope) (int64, error) {
	if !scope.IsValid() {
		return 0, coretenant.ErrInvalidScope
	}

	query := `
		SELECT COALESCE(SUM(size_bytes), 0)
		FROM asset_objects
		WHERE organization_id = $1 AND deleted_at IS NULL
	`

	var total int64
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, scope.OrganizationID()).Scan(&total)
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}
