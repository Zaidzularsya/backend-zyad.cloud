package repository

import (
	"context"
	"time"

	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/platform/database"
)

// MaxWebhookAttempts bounds retries; exhausted events stay in
// wa_webhook_events with their last error for manual reprocessing.
const MaxWebhookAttempts = 10

type InsertWebhookEventParams struct {
	EventID     string
	SessionName string
	EventType   string
	Payload     []byte
}

// WebhookEventRepository accesses wa_webhook_events, which is intentionally
// not under RLS (rows arrive before the organization is known). Only the
// webhook receiver and the worker use it.
type WebhookEventRepository interface {
	// Insert stores the event once; inserted is false for a duplicate event_id.
	Insert(ctx context.Context, params InsertWebhookEventParams) (inserted bool, err error)
	// ClaimDue leases up to limit unprocessed events whose retry time has
	// come, counting the attempt. A leased event is invisible to other
	// claimers until the lease expires, so a crashed worker's events retry.
	ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]domain.WebhookEvent, error)
	MarkProcessed(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, errorMessage string, nextRetryAt time.Time) error
}

type webhookEventRepository struct {
	db *database.Pool
}

func NewWebhookEventRepository(db *database.Pool) WebhookEventRepository {
	return &webhookEventRepository{db: db}
}

func (r *webhookEventRepository) Insert(ctx context.Context, params InsertWebhookEventParams) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO wa_webhook_events (event_id, session_name, event_type, payload)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (event_id) DO NOTHING
	`, params.EventID, params.SessionName, params.EventType, params.Payload)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *webhookEventRepository) ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]domain.WebhookEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	rows, err := r.db.Query(ctx, `
		WITH due AS (
			SELECT id
			FROM wa_webhook_events
			WHERE processed_at IS NULL
				AND next_retry_at <= now()
				AND attempts < $2
			ORDER BY received_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE wa_webhook_events e
		SET attempts = e.attempts + 1,
			next_retry_at = now() + make_interval(secs => $3)
		FROM due
		WHERE e.id = due.id
		RETURNING e.id, e.event_id, e.session_name, e.event_type, e.payload, e.attempts,
			COALESCE(e.error, ''), e.received_at, e.next_retry_at, e.processed_at
	`, limit, MaxWebhookAttempts, lease.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []domain.WebhookEvent{}
	for rows.Next() {
		var event domain.WebhookEvent
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.SessionName, &event.EventType, &event.Payload, &event.Attempts,
			&event.Error, &event.ReceivedAt, &event.NextRetryAt, &event.ProcessedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *webhookEventRepository) MarkProcessed(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE wa_webhook_events SET processed_at = now(), error = NULL WHERE id = $1
	`, id)
	return err
}

func (r *webhookEventRepository) MarkFailed(ctx context.Context, id string, errorMessage string, nextRetryAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE wa_webhook_events SET error = $2, next_retry_at = $3 WHERE id = $1
	`, id, errorMessage, nextRetryAt)
	return err
}
