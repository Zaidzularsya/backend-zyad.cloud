package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/platform/database"
)

type settingsRepository struct{ db *database.Pool }

func NewSettingsRepository(db *database.Pool) SettingsRepository { return &settingsRepository{db: db} }

func defaultSettings() domain.Settings {
	return domain.Settings{InvoiceLeadDays: 7, PaymentTermsDays: 7, DefaultChannels: []string{"email"}}
}

func (r *settingsRepository) Get(ctx context.Context, scope coretenant.Scope) (domain.Settings, error) {
	out := defaultSettings()
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var s domain.Settings
		err := tx.QueryRow(ctx, `
			SELECT invoice_lead_days, payment_terms_days, default_channels, COALESCE(default_sender_user_id::text, '')
			FROM receivable_settings WHERE organization_id = $1`, scope.OrganizationID()).
			Scan(&s.InvoiceLeadDays, &s.PaymentTermsDays, &s.DefaultChannels, &s.DefaultSenderUserID)
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		out = s
		return nil
	})
	return out, err
}

func (r *settingsRepository) Upsert(ctx context.Context, scope coretenant.Scope, s domain.Settings, by string) (domain.Settings, error) {
	channels := s.DefaultChannels
	if len(channels) == 0 {
		channels = []string{"email"}
	}
	var out domain.Settings
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO receivable_settings (organization_id, invoice_lead_days, payment_terms_days, default_channels,
				default_sender_user_id, updated_by, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())
			ON CONFLICT (organization_id) DO UPDATE SET
				invoice_lead_days = EXCLUDED.invoice_lead_days, payment_terms_days = EXCLUDED.payment_terms_days,
				default_channels = EXCLUDED.default_channels, default_sender_user_id = EXCLUDED.default_sender_user_id,
				updated_by = EXCLUDED.updated_by, updated_at = now()
			RETURNING invoice_lead_days, payment_terms_days, default_channels, COALESCE(default_sender_user_id::text, '')`,
			scope.OrganizationID(), s.InvoiceLeadDays, s.PaymentTermsDays, channels,
			nullableString(s.DefaultSenderUserID), nullableString(by)).
			Scan(&out.InvoiceLeadDays, &out.PaymentTermsDays, &out.DefaultChannels, &out.DefaultSenderUserID)
	})
	return out, err
}
