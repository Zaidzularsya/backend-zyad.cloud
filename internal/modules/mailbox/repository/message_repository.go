package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mailbox/domain"
	"zyad.cloud/internal/platform/database"
)

type messageRepository struct {
	db *database.Pool
}

func NewMessageRepository(db *database.Pool) MessageRepository {
	return &messageRepository{db: db}
}

const messageColumns = `
	m.id, m.mailbox_id, m.direction, m.status, m.message_id,
	COALESCE(m.in_reply_to, ''), COALESCE(m.references_header, ''),
	m.from_address, COALESCE(m.from_name, ''), m.to_addresses, m.cc_addresses, m.bcc_addresses,
	m.subject, m.snippet, m.body_html, m.body_text, COALESCE(m.error, ''),
	COALESCE(m.related_entity_type, ''), COALESCE(m.related_entity_id::text, ''),
	m.sent_at, COALESCE(m.created_by::text, ''), m.created_at
`

func scanMessage(row pgx.Row) (domain.Message, error) {
	var m domain.Message
	var direction, status, entityType string
	err := row.Scan(
		&m.ID, &m.MailboxID, &direction, &status, &m.MessageID,
		&m.InReplyTo, &m.References,
		&m.FromAddress, &m.FromName, &m.To, &m.Cc, &m.Bcc,
		&m.Subject, &m.Snippet, &m.BodyHTML, &m.BodyText, &m.Error,
		&entityType, &m.RelatedEntityID,
		&m.SentAt, &m.CreatedBy, &m.CreatedAt,
	)
	if err != nil {
		return domain.Message{}, err
	}
	m.Direction = domain.Direction(direction)
	m.Status = domain.MessageStatus(status)
	m.RelatedEntityType = domain.RelatedEntityType(entityType)
	return m, nil
}

// participants is every address of the message, lowercased, for the GIN
// "emails with X" filter.
func participants(from string, lists ...[]string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(address string) {
		address = strings.ToLower(strings.TrimSpace(address))
		if address != "" && !seen[address] {
			seen[address] = true
			out = append(out, address)
		}
	}
	add(from)
	for _, list := range lists {
		for _, address := range list {
			add(address)
		}
	}
	return out
}

func emptyIfNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (r *messageRepository) CreateOutbound(ctx context.Context, scope coretenant.Scope, p CreateOutboundParams) (domain.Message, bool, error) {
	var message domain.Message
	created := true
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		message, err = scanMessage(tx.QueryRow(ctx, `
			WITH inserted AS (
				INSERT INTO mail_messages AS m (
					organization_id, mailbox_id, direction, status, message_id, in_reply_to, references_header,
					from_address, from_name, to_addresses, cc_addresses, bcc_addresses, participants,
					subject, snippet, body_html, body_text, client_request_id,
					related_entity_type, related_entity_id, created_by
				) VALUES (
					$1, $2, 'outbound', 'queued', $3, $4, $5,
					$6, $7, $8, $9, $10, $11,
					$12, $13, $14, $15, $16,
					$17, $18, $19
				)
				ON CONFLICT (mailbox_id, client_request_id) DO NOTHING
				RETURNING *
			)
			SELECT `+messageColumns+` FROM inserted m`,
			scope.OrganizationID(), p.MailboxID, p.MessageID, nullableString(p.InReplyTo), nullableString(p.References),
			p.FromAddress, nullableString(p.FromName), emptyIfNil(p.To), emptyIfNil(p.Cc), emptyIfNil(p.Bcc),
			participants(p.FromAddress, p.To, p.Cc, p.Bcc),
			p.Subject, p.Snippet, p.BodyHTML, p.BodyText, nullableString(p.ClientRequestID),
			nullableString(string(p.RelatedEntityType)), nullableString(p.RelatedEntityID), nullableString(p.CreatedBy),
		))
		if !errors.Is(err, pgx.ErrNoRows) || p.ClientRequestID == "" {
			return err
		}
		// Same request sent twice: hand back the first one.
		created = false
		message, err = scanMessage(tx.QueryRow(ctx, `
			SELECT `+messageColumns+` FROM mail_messages m
			WHERE m.organization_id = $1 AND m.mailbox_id = $2 AND m.client_request_id = $3
		`, scope.OrganizationID(), p.MailboxID, p.ClientRequestID))
		return err
	})
	if err != nil {
		return domain.Message{}, false, err
	}
	return message, created, nil
}

func (r *messageRepository) AddAttachment(ctx context.Context, scope coretenant.Scope, messageID, assetObjectID string) error {
	return withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO mail_message_attachments (organization_id, message_id, asset_object_id)
			VALUES ($1, $2, $3)
		`, scope.OrganizationID(), messageID, assetObjectID)
		return err
	})
}

func (r *messageRepository) MarkSent(ctx context.Context, scope coretenant.Scope, id string, sentAt time.Time) error {
	return withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE mail_messages SET status = 'sent', sent_at = $3, error = NULL, updated_at = now()
			WHERE organization_id = $1 AND id = $2
		`, scope.OrganizationID(), id, sentAt)
		return err
	})
}

func (r *messageRepository) MarkFailed(ctx context.Context, scope coretenant.Scope, id, errorMessage string) error {
	return withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE mail_messages SET status = 'failed', error = $3, updated_at = now()
			WHERE organization_id = $1 AND id = $2
		`, scope.OrganizationID(), id, errorMessage)
		return err
	})
}

func (r *messageRepository) FailStaleQueued(ctx context.Context, scope coretenant.Scope, userID string, cutoff time.Time) error {
	return withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE mail_messages m SET status = 'failed', error = 'sending was interrupted', updated_at = now()
			FROM user_mailboxes b
			WHERE m.organization_id = $1 AND b.organization_id = $1 AND b.id = m.mailbox_id AND b.user_id = $2
				AND m.status = 'queued' AND m.created_at < $3
		`, scope.OrganizationID(), userID, cutoff)
		return err
	})
}

func (r *messageRepository) List(ctx context.Context, scope coretenant.Scope, filter MessageListFilter) ([]domain.Message, int64, error) {
	where := []string{"m.organization_id = $1", "b.user_id = $2"}
	args := []any{scope.OrganizationID(), filter.UserID}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if filter.MailboxID != "" {
		add("m.mailbox_id = $%d", filter.MailboxID)
	}
	if filter.Participant != "" {
		add("m.participants @> ARRAY[$%d]::text[]", strings.ToLower(strings.TrimSpace(filter.Participant)))
	}
	if filter.Direction != "" {
		add("m.direction = $%d", string(filter.Direction))
	}
	if filter.RelatedEntityType != "" && filter.RelatedEntityID != "" {
		add("m.related_entity_type = $%d", string(filter.RelatedEntityType))
		add("m.related_entity_id = $%d", filter.RelatedEntityID)
	}
	from := ` FROM mail_messages m
		JOIN user_mailboxes b ON b.id = m.mailbox_id AND b.organization_id = m.organization_id
		WHERE ` + strings.Join(where, " AND ")

	messages := []domain.Message{}
	var total int64
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*)`+from, args...).Scan(&total); err != nil {
			return err
		}
		pageArgs := append(append([]any{}, args...), filter.Limit, filter.Offset)
		rows, err := tx.Query(ctx, `SELECT `+messageColumns+from+
			fmt.Sprintf(` ORDER BY COALESCE(m.sent_at, m.created_at) DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2),
			pageArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			message, err := scanMessage(rows)
			if err != nil {
				return err
			}
			messages = append(messages, message)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return attachAttachments(ctx, tx, scope, messages)
	})
	if err != nil {
		return nil, 0, err
	}
	return messages, total, nil
}

func (r *messageRepository) GetForUser(ctx context.Context, scope coretenant.Scope, userID, id string) (domain.Message, error) {
	var message domain.Message
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		message, err = scanMessage(tx.QueryRow(ctx, `
			SELECT `+messageColumns+` FROM mail_messages m
			JOIN user_mailboxes b ON b.id = m.mailbox_id AND b.organization_id = m.organization_id
			WHERE m.organization_id = $1 AND b.user_id = $2 AND m.id = $3
		`, scope.OrganizationID(), userID, id))
		if err != nil {
			return err
		}
		list := []domain.Message{message}
		if err := attachAttachments(ctx, tx, scope, list); err != nil {
			return err
		}
		message = list[0]
		return nil
	})
	return message, err
}

// attachAttachments fills Attachments of each message in place. Files
// already removed from storage (asset_objects.deleted_at) are skipped.
func attachAttachments(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, messages []domain.Message) error {
	if len(messages) == 0 {
		return nil
	}
	ids := make([]string, 0, len(messages))
	index := map[string]int{}
	for i, message := range messages {
		ids = append(ids, message.ID)
		index[message.ID] = i
	}
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.message_id, a.asset_object_id, o.filename, o.mime_type, o.size_bytes
		FROM mail_message_attachments a
		JOIN asset_objects o ON o.id = a.asset_object_id AND o.organization_id = a.organization_id AND o.deleted_at IS NULL
		WHERE a.organization_id = $1 AND a.message_id = ANY($2::uuid[])
		ORDER BY a.created_at
	`, scope.OrganizationID(), ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a domain.Attachment
		if err := rows.Scan(&a.ID, &a.MessageID, &a.AssetObjectID, &a.Filename, &a.MimeType, &a.SizeBytes); err != nil {
			return err
		}
		i := index[a.MessageID]
		messages[i].Attachments = append(messages[i].Attachments, a)
	}
	return rows.Err()
}

func (r *messageRepository) AttachmentAssetIDs(ctx context.Context, scope coretenant.Scope, mailboxID string) ([]string, error) {
	ids := []string{}
	err := withTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.asset_object_id FROM mail_message_attachments a
			JOIN mail_messages m ON m.id = a.message_id AND m.organization_id = a.organization_id
			WHERE a.organization_id = $1 AND m.mailbox_id = $2
		`, scope.OrganizationID(), mailboxID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	return ids, err
}
