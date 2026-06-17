package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
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

func (r *integrationRepository) CreateIntegration(ctx context.Context, scope coretenant.Scope, params CreateIntegrationParams) (domain.LandingLeadIntegration, error) {
	if !scope.IsValid() {
		return domain.LandingLeadIntegration{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_lead_integrations (
			organization_id, name, type, credentials, event_filters, is_active, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		) RETURNING
			id, name, type, credentials, event_filters, is_active, created_by, updated_by, created_at, updated_at
	`

	var i domain.LandingLeadIntegration
	var createdBy, updatedBy *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var cb interface{} = nil
		if params.CreatedBy != "" {
			cb = params.CreatedBy
		}

		err := tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.Name,
			params.Type,
			params.Credentials,
			params.EventFilters,
			params.IsActive,
			cb,
		).Scan(
			&i.ID, &i.Name, &i.Type, &i.Credentials, &i.EventFilters, &i.IsActive,
			&createdBy, &updatedBy, &i.CreatedAt, &i.UpdatedAt,
		)
		return err
	})

	if err != nil {
		return domain.LandingLeadIntegration{}, err
	}
	i.OrganizationID = scope.OrganizationID()
	if createdBy != nil { i.CreatedBy = *createdBy }
	if updatedBy != nil { i.UpdatedBy = *updatedBy }

	return i, nil
}

func (r *integrationRepository) GetIntegration(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingLeadIntegration, error) {
	if !scope.IsValid() {
		return domain.LandingLeadIntegration{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, name, type, credentials, event_filters, is_active, created_by, updated_by, created_at, updated_at
		FROM landing_lead_integrations
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	var i domain.LandingLeadIntegration
	var createdBy, updatedBy *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&i.ID, &i.Name, &i.Type, &i.Credentials, &i.EventFilters, &i.IsActive,
			&createdBy, &updatedBy, &i.CreatedAt, &i.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingLeadIntegration{}, err
	}
	i.OrganizationID = scope.OrganizationID()
	if createdBy != nil { i.CreatedBy = *createdBy }
	if updatedBy != nil { i.UpdatedBy = *updatedBy }

	return i, nil
}

func (r *integrationRepository) ListIntegrations(ctx context.Context, scope coretenant.Scope) ([]domain.LandingLeadIntegration, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, name, type, credentials, event_filters, is_active, created_by, updated_by, created_at, updated_at
		FROM landing_lead_integrations
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	var integrations []domain.LandingLeadIntegration

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var i domain.LandingLeadIntegration
			var createdBy, updatedBy *string

			err := rows.Scan(
				&i.ID, &i.Name, &i.Type, &i.Credentials, &i.EventFilters, &i.IsActive,
				&createdBy, &updatedBy, &i.CreatedAt, &i.UpdatedAt,
			)
			if err != nil {
				return err
			}
			i.OrganizationID = scope.OrganizationID()
			if createdBy != nil { i.CreatedBy = *createdBy }
			if updatedBy != nil { i.UpdatedBy = *updatedBy }
			integrations = append(integrations, i)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return integrations, nil
}

func (r *integrationRepository) UpdateIntegration(ctx context.Context, scope coretenant.Scope, id string, params UpdateIntegrationParams) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE landing_lead_integrations
		SET name = $1, credentials = $2, event_filters = $3, is_active = $4, updated_by = $5, updated_at = NOW()
		WHERE id = $6 AND organization_id = $7 AND deleted_at IS NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var ub interface{} = nil
		if params.UpdatedBy != "" {
			ub = params.UpdatedBy
		}

		cmdTag, err := tx.Exec(ctx, query,
			params.Name, params.Credentials, params.EventFilters, params.IsActive, ub,
			id, scope.OrganizationID(),
		)
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *integrationRepository) DeleteIntegration(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE landing_lead_integrations
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

func (r *integrationRepository) CreateDeliveryLog(ctx context.Context, scope coretenant.Scope, params CreateDeliveryLogParams) (domain.LandingLeadDeliveryLog, error) {
	if !scope.IsValid() {
		return domain.LandingLeadDeliveryLog{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_lead_delivery_logs (
			organization_id, integration_id, submission_id, status, next_retry_at
		) VALUES (
			$1, $2, $3, $4, $5
		) RETURNING
			id, integration_id, submission_id, status, response_payload, error_message, attempts, next_retry_at, created_at, updated_at
	`

	var d domain.LandingLeadDeliveryLog

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.IntegrationID,
			params.SubmissionID,
			params.Status,
			params.NextRetryAt,
		).Scan(
			&d.ID, &d.IntegrationID, &d.SubmissionID, &d.Status,
			&d.ResponsePayload, &d.ErrorMessage, &d.Attempts, &d.NextRetryAt,
			&d.CreatedAt, &d.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingLeadDeliveryLog{}, err
	}
	d.OrganizationID = scope.OrganizationID()

	return d, nil
}

func (r *integrationRepository) ListDeliveryLogs(ctx context.Context, scope coretenant.Scope, submissionID string) ([]domain.LandingLeadDeliveryLog, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, integration_id, submission_id, status, response_payload, error_message, attempts, next_retry_at, created_at, updated_at
		FROM landing_lead_delivery_logs
		WHERE organization_id = $1 AND submission_id = $2
		ORDER BY created_at DESC
	`

	var logs []domain.LandingLeadDeliveryLog

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID(), submissionID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var d domain.LandingLeadDeliveryLog
			err := rows.Scan(
				&d.ID, &d.IntegrationID, &d.SubmissionID, &d.Status,
				&d.ResponsePayload, &d.ErrorMessage, &d.Attempts, &d.NextRetryAt,
				&d.CreatedAt, &d.UpdatedAt,
			)
			if err != nil {
				return err
			}
			d.OrganizationID = scope.OrganizationID()
			logs = append(logs, d)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return logs, nil
}

func (r *integrationRepository) ClaimPendingDeliveries(ctx context.Context, limit int) ([]domain.LandingLeadDeliveryLog, error) {
	// Worker operation, bypasses RLS
	query := `
		WITH pending AS (
			SELECT id
			FROM landing_lead_delivery_logs
			WHERE status = 'pending'
			  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE landing_lead_delivery_logs d
		SET status = 'pending',
			attempts = attempts + 1,
			updated_at = NOW()
		FROM pending p
		WHERE d.id = p.id
		RETURNING d.id, d.organization_id, d.integration_id, d.submission_id, d.status, d.response_payload, d.error_message, d.attempts, d.next_retry_at, d.created_at, d.updated_at
	`

	var logs []domain.LandingLeadDeliveryLog

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var d domain.LandingLeadDeliveryLog
		err := rows.Scan(
			&d.ID, &d.OrganizationID, &d.IntegrationID, &d.SubmissionID, &d.Status,
			&d.ResponsePayload, &d.ErrorMessage, &d.Attempts, &d.NextRetryAt,
			&d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, d)
	}

	return logs, rows.Err()
}

func (r *integrationRepository) UpdateDeliveryLogStatus(ctx context.Context, id string, params UpdateDeliveryLogParams) error {
	// Worker operation, bypasses RLS
	query := `
		UPDATE landing_lead_delivery_logs
		SET status = $1, response_payload = $2, error_message = $3, next_retry_at = $4, updated_at = NOW()
		WHERE id = $5
	`

	cmdTag, err := r.db.Exec(ctx, query, params.Status, params.ResponsePayload, params.ErrorMessage, params.NextRetryAt, id)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
