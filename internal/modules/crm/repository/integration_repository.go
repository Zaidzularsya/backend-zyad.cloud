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

type integrationRepository struct {
	db *database.Pool
}

func NewIntegrationRepository(db *database.Pool) IntegrationRepository {
	return &integrationRepository{db: db}
}

func (r *integrationRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

const integrationColumns = `
	id, organization_id, provider, name, config, secret_encrypted, is_active,
	connected_at, last_synced_at, created_by, updated_by, created_at, updated_at, deleted_at
`

func scanIntegration(row pgx.Row) (domain.Integration, error) {
	var integ domain.Integration
	var secretEncrypted *string
	var createdBy, updatedBy *string
	var provider string

	err := row.Scan(
		&integ.ID, &integ.OrganizationID, &provider, &integ.Name, &integ.Config, &secretEncrypted, &integ.IsActive,
		&integ.ConnectedAt, &integ.LastSyncedAt, &createdBy, &updatedBy, &integ.CreatedAt, &integ.UpdatedAt, &integ.DeletedAt,
	)
	if err != nil {
		return domain.Integration{}, err
	}

	integ.Provider = domain.IntegrationProvider(provider)
	if secretEncrypted != nil {
		integ.SecretEncrypted = *secretEncrypted
	}
	if createdBy != nil {
		integ.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		integ.UpdatedBy = *updatedBy
	}

	return integ, nil
}

func (r *integrationRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateIntegrationParams) (domain.Integration, error) {
	if !scope.IsValid() {
		return domain.Integration{}, coretenant.ErrInvalidScope
	}

	config := params.Config
	if config == nil {
		config = map[string]any{}
	}

	query := `
		INSERT INTO crm_integrations (
			organization_id, provider, name, config, secret_encrypted, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6
		) RETURNING ` + integrationColumns

	var integ domain.Integration
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		integ, scanErr = scanIntegration(tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			string(params.Provider),
			params.Name,
			config,
			nullableString(params.SecretEncrypted),
			nullableString(params.CreatedBy),
		))
		return scanErr
	})
	if err != nil {
		return domain.Integration{}, err
	}
	return integ, nil
}

func (r *integrationRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Integration, error) {
	if !scope.IsValid() {
		return domain.Integration{}, coretenant.ErrInvalidScope
	}

	query := `SELECT ` + integrationColumns + ` FROM crm_integrations WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	var integ domain.Integration
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		integ, scanErr = scanIntegration(tx.QueryRow(ctx, query, id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Integration{}, err
	}
	return integ, nil
}

func (r *integrationRepository) List(ctx context.Context, scope coretenant.Scope, filter IntegrationListFilter) ([]domain.Integration, int64, error) {
	if !scope.IsValid() {
		return nil, 0, coretenant.ErrInvalidScope
	}

	whereClauses := []string{"organization_id = $1"}
	args := []interface{}{scope.OrganizationID()}

	if !filter.IncludeDeleted {
		whereClauses = append(whereClauses, "deleted_at IS NULL")
	}
	if filter.Provider != "" {
		args = append(args, string(filter.Provider))
		whereClauses = append(whereClauses, fmt.Sprintf("provider = $%d", len(args)))
	}

	where := strings.Join(whereClauses, " AND ")
	countQuery := "SELECT COUNT(*) FROM crm_integrations WHERE " + where

	var integrations []domain.Integration
	var total int64

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			integrations = []domain.Integration{}
			return nil
		}

		query := "SELECT " + integrationColumns + " FROM crm_integrations WHERE " + where + " ORDER BY created_at DESC"
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
			integ, scanErr := scanIntegration(rows)
			if scanErr != nil {
				return scanErr
			}
			integrations = append(integrations, integ)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}

	return integrations, total, nil
}

func (r *integrationRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateIntegrationParams) (domain.Integration, error) {
	if !scope.IsValid() {
		return domain.Integration{}, coretenant.ErrInvalidScope
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
	if params.Config != nil {
		addSet("config", params.Config)
	}
	if params.SecretEncrypted != nil {
		addSet("secret_encrypted", nullableString(*params.SecretEncrypted))
	}
	if params.IsActive != nil {
		addSet("is_active", *params.IsActive)
	}
	if params.UpdatedBy != "" {
		addSet("updated_by", params.UpdatedBy)
	}

	args = append(args, id, scope.OrganizationID())
	query := "UPDATE crm_integrations SET " + strings.Join(setClauses, ", ") +
		fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", len(args)-1, len(args)) +
		integrationColumns

	var integ domain.Integration
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		integ, scanErr = scanIntegration(tx.QueryRow(ctx, query, args...))
		return scanErr
	})
	if err != nil {
		return domain.Integration{}, err
	}
	return integ, nil
}

func (r *integrationRepository) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_integrations
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

func (r *integrationRepository) Connect(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) (domain.Integration, error) {
	if !scope.IsValid() {
		return domain.Integration{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_integrations
		SET is_active = true, connected_at = NOW(), updated_by = $1, updated_at = NOW()
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
		RETURNING ` + integrationColumns

	var integ domain.Integration
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		integ, scanErr = scanIntegration(tx.QueryRow(ctx, query, nullableString(updatedBy), id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Integration{}, err
	}
	return integ, nil
}

func (r *integrationRepository) UpdateSecret(ctx context.Context, scope coretenant.Scope, id string, secretEncrypted string, updatedBy string) (domain.Integration, error) {
	if !scope.IsValid() {
		return domain.Integration{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_integrations
		SET secret_encrypted = $1, updated_by = $2, updated_at = NOW()
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL
		RETURNING ` + integrationColumns

	var integ domain.Integration
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		integ, scanErr = scanIntegration(tx.QueryRow(ctx, query, secretEncrypted, nullableString(updatedBy), id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Integration{}, err
	}
	return integ, nil
}
