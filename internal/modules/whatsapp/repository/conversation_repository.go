package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/platform/database"
)

type CreateConversationParams struct {
	SessionID         string
	ChatID            string
	PhoneNormalized   string
	ContactName       string
	RelatedEntityType domain.RelatedEntityType
	RelatedEntityID   string
	AssigneeUserID    string
}

type RecordMessageParams struct {
	WAHAMessageID string
	Direction     domain.MessageDirection
	Body          string
	Preview       string
	Status        domain.MessageStatus
	SentByUserID  string
	SentAt        time.Time
	Raw           map[string]any
}

// ConversationRepository manages wa_conversations and wa_messages (both RLS).
type ConversationRepository interface {
	// GetBySessionChat returns the conversation for a session chat, or pgx.ErrNoRows.
	GetBySessionChat(ctx context.Context, scope coretenant.Scope, sessionID, chatID string) (domain.Conversation, error)
	// Create inserts the conversation unless one already exists for
	// (session_id, chat_id); created is false when another writer won the race
	// and the existing row is returned.
	Create(ctx context.Context, scope coretenant.Scope, params CreateConversationParams) (conversation domain.Conversation, created bool, err error)
	// RecordMessage stores a message once per (conversation, WAHA message id)
	// and, only when newly inserted, advances the conversation's last message
	// and unread count (inbound only), all in one transaction.
	RecordMessage(ctx context.Context, scope coretenant.Scope, conversationID string, params RecordMessageParams) (message domain.Message, inserted bool, err error)
	// FindMessageByWAHAID looks up a message of the session by WAHA id, or pgx.ErrNoRows.
	FindMessageByWAHAID(ctx context.Context, scope coretenant.Scope, sessionID, wahaMessageID string) (domain.Message, error)
	// SetMessageStatus updates the status only if it is still from (the
	// caller decides whether the transition is allowed). updated is false
	// when the status changed concurrently.
	SetMessageStatus(ctx context.Context, scope coretenant.Scope, id string, from, to domain.MessageStatus) (updated bool, err error)
}

type conversationRepository struct {
	db *database.Pool
}

func NewConversationRepository(db *database.Pool) ConversationRepository {
	return &conversationRepository{db: db}
}

func (r *conversationRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

const conversationColumns = `
	id, organization_id, session_id, chat_id, phone_normalized, contact_name, related_entity_type,
	related_entity_id, assignee_user_id, last_message_at, last_message_preview, unread_count, status,
	created_at, updated_at
`

func scanConversation(row pgx.Row) (domain.Conversation, error) {
	var c domain.Conversation
	var phone, contactName, entityType, entityID, assignee, preview *string
	var status string
	err := row.Scan(
		&c.ID, &c.OrganizationID, &c.SessionID, &c.ChatID, &phone, &contactName, &entityType,
		&entityID, &assignee, &c.LastMessageAt, &preview, &c.UnreadCount, &status,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return domain.Conversation{}, err
	}
	c.PhoneNormalized = derefString(phone)
	c.ContactName = derefString(contactName)
	c.RelatedEntityType = domain.RelatedEntityType(derefString(entityType))
	c.RelatedEntityID = derefString(entityID)
	c.AssigneeUserID = derefString(assignee)
	c.LastMessagePreview = derefString(preview)
	c.Status = domain.ConversationStatus(status)
	return c, nil
}

const messageColumns = `
	id, organization_id, conversation_id, waha_message_id, direction, body, status, error,
	sent_by_user_id, sent_at, raw, created_at, updated_at
`

func scanMessage(row pgx.Row) (domain.Message, error) {
	var m domain.Message
	var wahaID, body, errText, sentBy *string
	var direction, status string
	var raw []byte
	err := row.Scan(
		&m.ID, &m.OrganizationID, &m.ConversationID, &wahaID, &direction, &body, &status, &errText,
		&sentBy, &m.SentAt, &raw, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return domain.Message{}, err
	}
	m.WAHAMessageID = derefString(wahaID)
	m.Direction = domain.MessageDirection(direction)
	m.Body = derefString(body)
	m.Status = domain.MessageStatus(status)
	m.Error = derefString(errText)
	m.SentByUserID = derefString(sentBy)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m.Raw); err != nil {
			return domain.Message{}, err
		}
	}
	return m, nil
}

func (r *conversationRepository) GetBySessionChat(ctx context.Context, scope coretenant.Scope, sessionID, chatID string) (domain.Conversation, error) {
	if !scope.IsValid() {
		return domain.Conversation{}, coretenant.ErrInvalidScope
	}

	var conversation domain.Conversation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		conversation, scanErr = scanConversation(tx.QueryRow(ctx, `
			SELECT `+conversationColumns+`
			FROM wa_conversations
			WHERE organization_id = $1 AND session_id = $2 AND chat_id = $3
		`, scope.OrganizationID(), sessionID, chatID))
		return scanErr
	})
	if err != nil {
		return domain.Conversation{}, err
	}
	return conversation, nil
}

func (r *conversationRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateConversationParams) (domain.Conversation, bool, error) {
	if !scope.IsValid() {
		return domain.Conversation{}, false, coretenant.ErrInvalidScope
	}

	var conversation domain.Conversation
	created := false
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		conversation, err = scanConversation(tx.QueryRow(ctx, `
			INSERT INTO wa_conversations (
				organization_id, session_id, chat_id, phone_normalized, contact_name,
				related_entity_type, related_entity_id, assignee_user_id
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (session_id, chat_id) DO NOTHING
			RETURNING `+conversationColumns,
			scope.OrganizationID(), params.SessionID, params.ChatID, nullableString(params.PhoneNormalized),
			nullableString(params.ContactName), nullableString(string(params.RelatedEntityType)),
			nullableString(params.RelatedEntityID), nullableString(params.AssigneeUserID),
		))
		if err == nil {
			created = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		conversation, err = scanConversation(tx.QueryRow(ctx, `
			SELECT `+conversationColumns+`
			FROM wa_conversations
			WHERE organization_id = $1 AND session_id = $2 AND chat_id = $3
		`, scope.OrganizationID(), params.SessionID, params.ChatID))
		return err
	})
	if err != nil {
		return domain.Conversation{}, false, err
	}
	return conversation, created, nil
}

func (r *conversationRepository) RecordMessage(ctx context.Context, scope coretenant.Scope, conversationID string, params RecordMessageParams) (domain.Message, bool, error) {
	if !scope.IsValid() {
		return domain.Message{}, false, coretenant.ErrInvalidScope
	}

	raw := params.Raw
	if raw == nil {
		raw = map[string]any{}
	}
	rawJSON, err := json.Marshal(raw)
	if err != nil {
		return domain.Message{}, false, err
	}

	var message domain.Message
	inserted := false
	err = r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		message, err = scanMessage(tx.QueryRow(ctx, `
			INSERT INTO wa_messages (
				organization_id, conversation_id, waha_message_id, direction, body, status,
				sent_by_user_id, sent_at, raw
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (conversation_id, waha_message_id) WHERE waha_message_id IS NOT NULL DO NOTHING
			RETURNING `+messageColumns,
			scope.OrganizationID(), conversationID, nullableString(params.WAHAMessageID), string(params.Direction),
			nullableString(params.Body), string(params.Status), nullableString(params.SentByUserID),
			params.SentAt, rawJSON,
		))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		inserted = true

		unreadDelta := 0
		if params.Direction == domain.MessageDirectionIn {
			unreadDelta = 1
		}
		_, err = tx.Exec(ctx, `
			UPDATE wa_conversations
			SET last_message_at = GREATEST(COALESCE(last_message_at, $3), $3),
				last_message_preview = CASE
					WHEN last_message_at IS NULL OR $3 >= last_message_at THEN $4
					ELSE last_message_preview
				END,
				unread_count = unread_count + $5,
				updated_at = now()
			WHERE id = $1 AND organization_id = $2
		`, conversationID, scope.OrganizationID(), params.SentAt, nullableString(truncatePreview(params.Preview)), unreadDelta)
		return err
	})
	if err != nil {
		return domain.Message{}, false, err
	}
	return message, inserted, nil
}

func (r *conversationRepository) FindMessageByWAHAID(ctx context.Context, scope coretenant.Scope, sessionID, wahaMessageID string) (domain.Message, error) {
	if !scope.IsValid() {
		return domain.Message{}, coretenant.ErrInvalidScope
	}

	var message domain.Message
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		message, scanErr = scanMessage(tx.QueryRow(ctx, `
			SELECT m.id, m.organization_id, m.conversation_id, m.waha_message_id, m.direction, m.body, m.status,
				m.error, m.sent_by_user_id, m.sent_at, m.raw, m.created_at, m.updated_at
			FROM wa_messages m
			JOIN wa_conversations c ON c.id = m.conversation_id AND c.organization_id = m.organization_id
			WHERE m.organization_id = $1 AND c.session_id = $2 AND m.waha_message_id = $3
			LIMIT 1
		`, scope.OrganizationID(), sessionID, wahaMessageID))
		return scanErr
	})
	if err != nil {
		return domain.Message{}, err
	}
	return message, nil
}

func (r *conversationRepository) SetMessageStatus(ctx context.Context, scope coretenant.Scope, id string, from, to domain.MessageStatus) (bool, error) {
	if !scope.IsValid() {
		return false, coretenant.ErrInvalidScope
	}

	updated := false
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE wa_messages
			SET status = $4, updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND status = $3
		`, id, scope.OrganizationID(), string(from), string(to))
		if err != nil {
			return err
		}
		updated = tag.RowsAffected() == 1
		return nil
	})
	return updated, err
}

// truncatePreview keeps last_message_preview within varchar(200) without
// splitting a UTF-8 character.
func truncatePreview(text string) string {
	runes := []rune(text)
	if len(runes) <= 200 {
		return text
	}
	return string(runes[:197]) + "..."
}
