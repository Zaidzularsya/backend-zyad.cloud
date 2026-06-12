package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/platform/database"
)

const outboxEventSelectColumns = `
	id::text,
	event_type,
	COALESCE(organization_id::text, ''),
	COALESCE(user_id::text, ''),
	recipient,
	payload,
	COALESCE(locale, ''),
	status,
	attempts,
	max_attempts,
	next_retry_at,
	COALESCE(error_message, ''),
	processed_at,
	created_at,
	updated_at
`

const outboxEventReturningColumns = `
	e.id::text,
	e.event_type,
	COALESCE(e.organization_id::text, ''),
	COALESCE(e.user_id::text, ''),
	e.recipient,
	e.payload,
	COALESCE(e.locale, ''),
	e.status,
	e.attempts,
	e.max_attempts,
	e.next_retry_at,
	COALESCE(e.error_message, ''),
	e.processed_at,
	e.created_at,
	e.updated_at
`

type OutboxRepository struct {
	db *database.Pool
}

func NewOutboxRepository(db *database.Pool) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func (r *OutboxRepository) Enqueue(ctx context.Context, event *domain.OutboxEvent) error {
	recipient, err := encodeRecipient(event.Recipient)
	if err != nil {
		return err
	}
	payload, err := encodeMap(event.Payload)
	if err != nil {
		return err
	}
	if event.MaxAttempts == 0 {
		event.MaxAttempts = 3
	}

	err = r.db.QueryRow(ctx, `
		INSERT INTO notification_outbox_events (
			id,
			event_type,
			organization_id,
			user_id,
			recipient,
			payload,
			locale,
			status,
			attempts,
			max_attempts,
			next_retry_at,
			error_message,
			created_at,
			updated_at
		)
		VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()),
			$2,
			NULLIF($3, '')::uuid,
			NULLIF($4, '')::uuid,
			$5::jsonb,
			$6::jsonb,
			NULLIF($7, ''),
			'pending',
			0,
			$8,
			NULL,
			NULL,
			now(),
			now()
		)
		ON CONFLICT (id) DO NOTHING
		RETURNING `+outboxEventSelectColumns,
		event.ID,
		event.EventType,
		event.OrganizationID,
		event.UserID,
		string(recipient),
		string(payload),
		event.Locale,
		event.MaxAttempts,
	).Scan(outboxEventScanDest(event)...)
	if err == pgx.ErrNoRows {
		return nil
	}
	return err
}

func (r *OutboxRepository) ClaimPending(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		WITH claimed AS (
			SELECT id
			FROM notification_outbox_events
			WHERE status IN ('pending', 'failed')
				AND attempts < max_attempts
				AND (next_retry_at IS NULL OR next_retry_at <= now())
			ORDER BY COALESCE(next_retry_at, created_at), created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE notification_outbox_events e
		SET status = 'processing',
			updated_at = now()
		FROM claimed
		WHERE e.id = claimed.id
		RETURNING `+outboxEventReturningColumns,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]domain.OutboxEvent, 0)
	for rows.Next() {
		var event domain.OutboxEvent
		if err := rows.Scan(outboxEventScanDest(&event)...); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return events, nil
}

func (r *OutboxRepository) MarkSucceeded(ctx context.Context, id string) error {
	return r.updateStatus(ctx, `
		UPDATE notification_outbox_events
		SET status = 'succeeded',
			error_message = NULL,
			processed_at = now(),
			updated_at = now()
		WHERE id = $1
	`, id)
}

func (r *OutboxRepository) MarkFailed(ctx context.Context, id string, errorMessage string, nextRetryAt *time.Time) error {
	return r.updateStatus(ctx, `
		UPDATE notification_outbox_events
		SET status = 'failed',
			attempts = LEAST(attempts + 1, max_attempts),
			next_retry_at = $3,
			error_message = NULLIF($2, ''),
			updated_at = now()
		WHERE id = $1
			AND status = 'processing'
	`, id, errorMessage, nextRetryAt)
}

func (r *OutboxRepository) MarkDead(ctx context.Context, id string, errorMessage string) error {
	return r.updateStatus(ctx, `
		UPDATE notification_outbox_events
		SET status = 'dead',
			attempts = max_attempts,
			next_retry_at = NULL,
			error_message = NULLIF($2, ''),
			processed_at = now(),
			updated_at = now()
		WHERE id = $1
	`, id, errorMessage)
}

func (r *OutboxRepository) FindByID(ctx context.Context, id string) (domain.OutboxEvent, error) {
	var event domain.OutboxEvent
	err := r.db.QueryRow(ctx, `
		SELECT `+outboxEventSelectColumns+`
		FROM notification_outbox_events
		WHERE id = $1
	`, id).Scan(outboxEventScanDest(&event)...)
	if err != nil {
		return domain.OutboxEvent{}, err
	}
	return event, nil
}

func (r *OutboxRepository) updateStatus(ctx context.Context, query string, args ...any) error {
	result, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func outboxEventScanDest(event *domain.OutboxEvent) []any {
	var nextRetryAt sql.NullTime
	var processedAt sql.NullTime

	return []any{
		&event.ID,
		&event.EventType,
		&event.OrganizationID,
		&event.UserID,
		(*recipientScanner)(&event.Recipient),
		(*mapScanner)(&event.Payload),
		&event.Locale,
		(*outboxStatusScanner)(&event.Status),
		&event.Attempts,
		&event.MaxAttempts,
		&nullableTimeScanner{value: &nextRetryAt, assign: &event.NextRetryAt},
		&event.ErrorMessage,
		&nullableTimeScanner{value: &processedAt, assign: &event.ProcessedAt},
		&event.CreatedAt,
		&event.UpdatedAt,
	}
}

func encodeRecipient(recipient domain.NotificationRecipient) ([]byte, error) {
	return json.Marshal(recipient)
}

type recipientScanner domain.NotificationRecipient

func (s *recipientScanner) Scan(value any) error {
	data, err := scanBytes(value)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		*s = recipientScanner(domain.NotificationRecipient{})
		return nil
	}
	var recipient domain.NotificationRecipient
	if err := json.Unmarshal(data, &recipient); err != nil {
		return fmt.Errorf("scan recipient: %w", err)
	}
	*s = recipientScanner(recipient)
	return nil
}

type outboxStatusScanner domain.OutboxStatus

func (s *outboxStatusScanner) Scan(value any) error {
	str, err := scanString(value)
	if err != nil {
		return err
	}
	*s = outboxStatusScanner(domain.OutboxStatus(str))
	return nil
}
