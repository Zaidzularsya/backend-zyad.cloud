package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
)

type crmSettingsRepository struct{ db *database.Pool }

func NewCRMSettingsRepository(db *database.Pool) CRMSettingsRepository {
	return &crmSettingsRepository{db: db}
}

func (r *crmSettingsRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID()); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *crmSettingsRepository) Get(ctx context.Context, scope coretenant.Scope) (CRMSettings, error) {
	if !scope.IsValid() {
		return CRMSettings{}, coretenant.ErrInvalidScope
	}
	s := CRMSettings{LeadPlaybookEnabled: true}
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT lead_playbook_enabled, updated_at FROM crm_settings WHERE organization_id = $1`,
			scope.OrganizationID()).Scan(&s.LeadPlaybookEnabled, &s.UpdatedAt)
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	})
	return s, err
}

func (r *crmSettingsRepository) Update(ctx context.Context, scope coretenant.Scope, enabled bool, updatedBy string) (CRMSettings, error) {
	if !scope.IsValid() {
		return CRMSettings{}, coretenant.ErrInvalidScope
	}
	var s CRMSettings
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO crm_settings (organization_id, lead_playbook_enabled, updated_by, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (organization_id) DO UPDATE
				SET lead_playbook_enabled = EXCLUDED.lead_playbook_enabled, updated_by = EXCLUDED.updated_by, updated_at = NOW()
			RETURNING lead_playbook_enabled, updated_at`,
			scope.OrganizationID(), enabled, nullableString(updatedBy)).Scan(&s.LeadPlaybookEnabled, &s.UpdatedAt)
	})
	return s, err
}
