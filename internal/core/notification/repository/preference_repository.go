package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/platform/database"
)

const preferenceSelectColumns = `
	id,
	user_id::text,
	COALESCE(organization_id::text, ''),
	event_type,
	channel,
	is_enabled,
	created_at,
	updated_at
`

type PreferenceRepository struct {
	db *database.Pool
}

func NewPreferenceRepository(db *database.Pool) *PreferenceRepository {
	return &PreferenceRepository{db: db}
}

func (r *PreferenceRepository) GetUserPreferences(ctx context.Context, userID string, organizationID string) ([]domain.NotificationPreference, error) {
	var (
		rows pgx.Rows
		err  error
	)

	if organizationID == "" {
		rows, err = r.db.Query(ctx, `
			SELECT `+preferenceSelectColumns+`
			FROM notification_preferences
			WHERE user_id = $1
				AND organization_id IS NULL
			ORDER BY event_type ASC, channel ASC
		`, userID)
	} else {
		rows, err = r.db.Query(ctx, `
			SELECT `+preferenceSelectColumns+`
			FROM notification_preferences
			WHERE user_id = $1
				AND organization_id = $2
			ORDER BY event_type ASC, channel ASC
		`, userID, organizationID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	preferences := make([]domain.NotificationPreference, 0)
	for rows.Next() {
		var preference domain.NotificationPreference
		if err := rows.Scan(preferenceScanDest(&preference)...); err != nil {
			return nil, err
		}
		preferences = append(preferences, preference)
	}

	return preferences, rows.Err()
}

func (r *PreferenceRepository) UpsertPreference(ctx context.Context, preference *domain.NotificationPreference) error {
	return upsertPreference(ctx, r.db, preference)
}

func (r *PreferenceRepository) BulkUpsertPreferences(ctx context.Context, preferences []domain.NotificationPreference) error {
	if len(preferences) == 0 {
		return nil
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for i := range preferences {
		if err := upsertPreference(ctx, tx, &preferences[i]); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *PreferenceRepository) IsEnabled(ctx context.Context, userID string, organizationID string, eventType string, channel domain.Channel) (bool, error) {
	var enabled bool

	var err error
	if organizationID == "" {
		err = r.db.QueryRow(ctx, `
			SELECT is_enabled
			FROM notification_preferences
			WHERE user_id = $1
				AND organization_id IS NULL
				AND event_type = $2
				AND channel = $3
			LIMIT 1
		`, userID, eventType, string(channel)).Scan(&enabled)
	} else {
		err = r.db.QueryRow(ctx, `
			SELECT is_enabled
			FROM notification_preferences
			WHERE user_id = $1
				AND event_type = $2
				AND channel = $3
				AND (organization_id = $4 OR organization_id IS NULL)
			ORDER BY organization_id IS NULL ASC
			LIMIT 1
		`, userID, eventType, string(channel), organizationID).Scan(&enabled)
	}
	if err == pgx.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	return enabled, nil
}

type preferenceQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func upsertPreference(ctx context.Context, querier preferenceQuerier, preference *domain.NotificationPreference) error {
	if preference.OrganizationID == "" {
		return querier.QueryRow(ctx, `
			INSERT INTO notification_preferences (
				user_id,
				organization_id,
				event_type,
				channel,
				is_enabled,
				created_at,
				updated_at
			)
			VALUES ($1, NULL, $2, $3, $4, now(), now())
			ON CONFLICT (user_id, event_type, channel)
				WHERE organization_id IS NULL
			DO UPDATE SET
				is_enabled = EXCLUDED.is_enabled,
				updated_at = now()
			RETURNING `+preferenceSelectColumns,
			preference.UserID,
			preference.EventType,
			string(preference.Channel),
			preference.IsEnabled,
		).Scan(preferenceScanDest(preference)...)
	}

	return querier.QueryRow(ctx, `
		INSERT INTO notification_preferences (
			user_id,
			organization_id,
			event_type,
			channel,
			is_enabled,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, now(), now())
		ON CONFLICT (user_id, organization_id, event_type, channel)
			WHERE organization_id IS NOT NULL
		DO UPDATE SET
			is_enabled = EXCLUDED.is_enabled,
			updated_at = now()
		RETURNING `+preferenceSelectColumns,
		preference.UserID,
		preference.OrganizationID,
		preference.EventType,
		string(preference.Channel),
		preference.IsEnabled,
	).Scan(preferenceScanDest(preference)...)
}

func preferenceScanDest(preference *domain.NotificationPreference) []any {
	return []any{
		&preference.ID,
		&preference.UserID,
		&preference.OrganizationID,
		&preference.EventType,
		(*channelScanner)(&preference.Channel),
		&preference.IsEnabled,
		&preference.CreatedAt,
		&preference.UpdatedAt,
	}
}
