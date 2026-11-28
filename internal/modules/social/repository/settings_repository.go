package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/social/domain"
	"zyad.cloud/internal/platform/database"
)

// SettingsRepository reads/writes social_tenant_settings (global table, no RLS):
// every query is keyed by the organization id from the verified tenant scope.
type SettingsRepository struct {
	db *database.Pool
}

func NewSettingsRepository(db *database.Pool) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) Get(ctx context.Context, organizationID string) (domain.TenantSettings, bool, error) {
	s, err := scanSettings(r.db.QueryRow(ctx, `
		SELECT organization_id::text, customer_claim_auto_offer, COALESCE(updated_by::text, ''), updated_at
		FROM social_tenant_settings
		WHERE organization_id = $1::uuid`, organizationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TenantSettings{}, false, nil
	}
	if err != nil {
		return domain.TenantSettings{}, false, fmt.Errorf("read social tenant settings: %w", err)
	}
	return s, true, nil
}

func (r *SettingsRepository) Upsert(ctx context.Context, s domain.TenantSettings) (domain.TenantSettings, error) {
	saved, err := scanSettings(r.db.QueryRow(ctx, `
		INSERT INTO social_tenant_settings (organization_id, customer_claim_auto_offer, updated_by, updated_at)
		VALUES ($1::uuid, $2, NULLIF($3, '')::uuid, now())
		ON CONFLICT (organization_id) DO UPDATE SET
			customer_claim_auto_offer = EXCLUDED.customer_claim_auto_offer,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
		RETURNING organization_id::text, customer_claim_auto_offer, COALESCE(updated_by::text, ''), updated_at`,
		s.OrganizationID, s.CustomerClaimAutoOffer, s.UpdatedBy))
	if err != nil {
		return domain.TenantSettings{}, fmt.Errorf("save social tenant settings: %w", err)
	}
	return saved, nil
}

func scanSettings(row pgx.Row) (domain.TenantSettings, error) {
	var s domain.TenantSettings
	var updatedAt time.Time
	if err := row.Scan(&s.OrganizationID, &s.CustomerClaimAutoOffer, &s.UpdatedBy, &updatedAt); err != nil {
		return domain.TenantSettings{}, err
	}
	s.UpdatedAt = &updatedAt
	return s, nil
}
