package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/platform/database"
)

const notificationLogSelectColumns = `
	id,
	COALESCE(event_id::text, ''),
	COALESCE(event_type, ''),
	COALESCE(template_id::text, ''),
	COALESCE(template_code, ''),
	COALESCE(template_version, 0),
	COALESCE(organization_id::text, ''),
	channel,
	recipient_type,
	COALESCE(recipient_user_id::text, ''),
	COALESCE(recipient_name_snapshot, ''),
	COALESCE(recipient_email_snapshot, ''),
	COALESCE(recipient_phone_snapshot, ''),
	destination,
	COALESCE(subject, ''),
	body,
	status,
	COALESCE(provider, ''),
	COALESCE(provider_message_id, ''),
	provider_response,
	attempts,
	max_attempts,
	next_retry_at,
	COALESCE(error_message, ''),
	sent_at,
	failed_at,
	cancelled_at,
	created_at,
	updated_at
`

type NotificationLogRepository struct {
	db *database.Pool
}

type NotificationLogListFilter struct {
	EventType       string
	TemplateCode    string
	OrganizationID  string
	RecipientUserID string
	Channel         domain.Channel
	Status          domain.LogStatus
	Limit           int
	Offset          int
}

func NewNotificationLogRepository(db *database.Pool) *NotificationLogRepository {
	return &NotificationLogRepository{db: db}
}

func (r *NotificationLogRepository) CreatePending(ctx context.Context, log *domain.NotificationLog) error {
	providerResponse, err := encodeMap(log.ProviderResponse)
	if err != nil {
		return err
	}

	if log.Status == "" {
		log.Status = domain.LogStatusPending
	}
	if log.RecipientType == "" {
		log.RecipientType = "user"
	}
	if log.MaxAttempts == 0 {
		log.MaxAttempts = 3
	}

	err = r.db.QueryRow(ctx, `
		INSERT INTO notification_logs (
			event_id,
			event_type,
			template_id,
			template_code,
			template_version,
			organization_id,
			channel,
			recipient_type,
			recipient_user_id,
			recipient_name_snapshot,
			recipient_email_snapshot,
			recipient_phone_snapshot,
			destination,
			subject,
			body,
			status,
			provider,
			provider_message_id,
			provider_response,
			attempts,
			max_attempts,
			next_retry_at,
			error_message,
			created_at,
			updated_at
		)
		VALUES (
			NULLIF($1, '')::uuid,
			NULLIF($2, ''),
			NULLIF($3, '')::uuid,
			NULLIF($4, ''),
			NULLIF($5, 0),
			NULLIF($6, '')::uuid,
			$7,
			$8,
			NULLIF($9, '')::uuid,
			NULLIF($10, ''),
			NULLIF($11, ''),
			NULLIF($12, ''),
			$13,
			NULLIF($14, ''),
			$15,
			$16,
			NULLIF($17, ''),
			NULLIF($18, ''),
			$19::jsonb,
			$20,
			$21,
			$22,
			NULLIF($23, ''),
			now(),
			now()
		)
		RETURNING `+notificationLogSelectColumns,
		log.EventID,
		log.EventType,
		log.TemplateID,
		log.TemplateCode,
		log.TemplateVersion,
		log.OrganizationID,
		string(log.Channel),
		log.RecipientType,
		log.RecipientUserID,
		log.RecipientNameSnapshot,
		log.RecipientEmailSnapshot,
		log.RecipientPhoneSnapshot,
		log.Destination,
		log.Subject,
		log.Body,
		string(log.Status),
		log.Provider,
		log.ProviderMessageID,
		string(providerResponse),
		log.Attempts,
		log.MaxAttempts,
		log.NextRetryAt,
		log.ErrorMessage,
	).Scan(notificationLogScanDest(log)...)
	if err != nil {
		return err
	}

	return nil
}

func (r *NotificationLogRepository) FindByID(ctx context.Context, id string) (domain.NotificationLog, error) {
	var log domain.NotificationLog
	err := r.db.QueryRow(ctx, `
		SELECT `+notificationLogSelectColumns+`
		FROM notification_logs
		WHERE id = $1
	`, id).Scan(notificationLogScanDest(&log)...)
	if err != nil {
		return domain.NotificationLog{}, err
	}
	return log, nil
}

func (r *NotificationLogRepository) List(ctx context.Context, filter NotificationLogListFilter) ([]domain.NotificationLog, error) {
	query := strings.Builder{}
	query.WriteString("SELECT ")
	query.WriteString(notificationLogSelectColumns)
	query.WriteString(" FROM notification_logs WHERE 1 = 1")

	args := make([]any, 0)
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if filter.EventType != "" {
		query.WriteString(" AND event_type = ")
		query.WriteString(addArg(filter.EventType))
	}
	if filter.TemplateCode != "" {
		query.WriteString(" AND template_code = ")
		query.WriteString(addArg(filter.TemplateCode))
	}
	if filter.OrganizationID != "" {
		query.WriteString(" AND organization_id = ")
		query.WriteString(addArg(filter.OrganizationID))
	}
	if filter.RecipientUserID != "" {
		query.WriteString(" AND recipient_user_id = ")
		query.WriteString(addArg(filter.RecipientUserID))
	}
	if filter.Channel != "" {
		query.WriteString(" AND channel = ")
		query.WriteString(addArg(string(filter.Channel)))
	}
	if filter.Status != "" {
		query.WriteString(" AND status = ")
		query.WriteString(addArg(string(filter.Status)))
	}

	query.WriteString(" ORDER BY created_at DESC")

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	query.WriteString(" LIMIT ")
	query.WriteString(addArg(limit))

	if filter.Offset > 0 {
		query.WriteString(" OFFSET ")
		query.WriteString(addArg(filter.Offset))
	}

	rows, err := r.db.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]domain.NotificationLog, 0)
	for rows.Next() {
		var log domain.NotificationLog
		if err := rows.Scan(notificationLogScanDest(&log)...); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}

	return logs, rows.Err()
}

func (r *NotificationLogRepository) MarkProcessing(ctx context.Context, id string) error {
	return r.updateStatus(ctx, id, `
		UPDATE notification_logs
		SET status = 'processing', updated_at = now()
		WHERE id = $1
			AND status IN ('pending', 'failed')
			AND attempts < max_attempts
	`, id)
}

func (r *NotificationLogRepository) MarkSent(ctx context.Context, id string, provider string, providerMessageID string, providerResponse map[string]any) error {
	response, err := encodeMap(providerResponse)
	if err != nil {
		return err
	}

	return r.updateStatus(ctx, id, `
		UPDATE notification_logs
		SET
			status = 'sent',
			provider = NULLIF($2, ''),
			provider_message_id = NULLIF($3, ''),
			provider_response = $4::jsonb,
			error_message = NULL,
			sent_at = now(),
			updated_at = now()
		WHERE id = $1
	`, id, provider, providerMessageID, string(response))
}

func (r *NotificationLogRepository) MarkFailed(ctx context.Context, id string, errorMessage string, nextRetryAt *time.Time) error {
	return r.updateStatus(ctx, id, `
		UPDATE notification_logs
		SET
			status = 'failed',
			attempts = LEAST(attempts + 1, max_attempts),
			next_retry_at = $3,
			error_message = NULLIF($2, ''),
			failed_at = now(),
			updated_at = now()
		WHERE id = $1
			AND status IN ('pending', 'processing', 'failed')
	`, id, errorMessage, nextRetryAt)
}

func (r *NotificationLogRepository) MarkDead(ctx context.Context, id string, errorMessage string) error {
	return r.updateStatus(ctx, id, `
		UPDATE notification_logs
		SET
			status = 'dead',
			attempts = max_attempts,
			next_retry_at = NULL,
			error_message = NULLIF($2, ''),
			failed_at = COALESCE(failed_at, now()),
			updated_at = now()
		WHERE id = $1
	`, id, errorMessage)
}

func (r *NotificationLogRepository) FindRetryable(ctx context.Context, limit int) ([]domain.NotificationLog, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := r.db.Query(ctx, `
		SELECT `+notificationLogSelectColumns+`
		FROM notification_logs
		WHERE status IN ('pending', 'failed')
			AND attempts < max_attempts
			AND (next_retry_at IS NULL OR next_retry_at <= now())
		ORDER BY COALESCE(next_retry_at, created_at), created_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]domain.NotificationLog, 0)
	for rows.Next() {
		var log domain.NotificationLog
		if err := rows.Scan(notificationLogScanDest(&log)...); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}

	return logs, rows.Err()
}

func (r *NotificationLogRepository) updateStatus(ctx context.Context, id string, query string, args ...any) error {
	result, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func notificationLogScanDest(log *domain.NotificationLog) []any {
	var nextRetryAt sql.NullTime
	var sentAt sql.NullTime
	var failedAt sql.NullTime
	var cancelledAt sql.NullTime

	return []any{
		&log.ID,
		&log.EventID,
		&log.EventType,
		&log.TemplateID,
		&log.TemplateCode,
		&log.TemplateVersion,
		&log.OrganizationID,
		(*channelScanner)(&log.Channel),
		&log.RecipientType,
		&log.RecipientUserID,
		&log.RecipientNameSnapshot,
		&log.RecipientEmailSnapshot,
		&log.RecipientPhoneSnapshot,
		&log.Destination,
		&log.Subject,
		&log.Body,
		(*logStatusScanner)(&log.Status),
		&log.Provider,
		&log.ProviderMessageID,
		(*mapScanner)(&log.ProviderResponse),
		&log.Attempts,
		&log.MaxAttempts,
		&nullableTimeScanner{value: &nextRetryAt, assign: &log.NextRetryAt},
		&log.ErrorMessage,
		&nullableTimeScanner{value: &sentAt, assign: &log.SentAt},
		&nullableTimeScanner{value: &failedAt, assign: &log.FailedAt},
		&nullableTimeScanner{value: &cancelledAt, assign: &log.CancelledAt},
		&log.CreatedAt,
		&log.UpdatedAt,
	}
}

type logStatusScanner domain.LogStatus

func (s *logStatusScanner) Scan(value any) error {
	str, err := scanString(value)
	if err != nil {
		return err
	}
	*s = logStatusScanner(domain.LogStatus(str))
	return nil
}
