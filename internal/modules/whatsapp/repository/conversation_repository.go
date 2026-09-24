package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

	GetByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Conversation, error)
	List(ctx context.Context, scope coretenant.Scope, filter ConversationListFilter) ([]domain.Conversation, int64, error)
	// LinkEntity links an unlinked conversation to a CRM entity (and sets the
	// assignee). It never overwrites an existing link; the current row is
	// returned either way.
	LinkEntity(ctx context.Context, scope coretenant.Scope, id string, entityType domain.RelatedEntityType, entityID, assigneeUserID string) (domain.Conversation, error)
	MarkRead(ctx context.Context, scope coretenant.Scope, id string) error
	Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateConversationParams) (domain.Conversation, error)
	// ClaimActivityDay sets crm_activity_on to day unless it already is;
	// claimed is true for exactly one caller per conversation and day.
	ClaimActivityDay(ctx context.Context, scope coretenant.Scope, id string, day time.Time) (claimed bool, err error)

	GetMessage(ctx context.Context, scope coretenant.Scope, id string) (domain.Message, error)
	// ListMessages returns up to limit messages older than beforeID (all when
	// empty), newest first.
	ListMessages(ctx context.Context, scope coretenant.Scope, conversationID, beforeID string, limit int) ([]domain.Message, error)
	// MarkMessageSent records the WAHA id of a pending outbound message. A row
	// the webhook processor may already have stored for the same WAHA id is
	// removed so the message is not listed twice.
	MarkMessageSent(ctx context.Context, scope coretenant.Scope, id, wahaMessageID string) (domain.Message, error)
	MarkMessageFailed(ctx context.Context, scope coretenant.Scope, id, errorMessage string) (domain.Message, error)
}

type ConversationListFilter struct {
	RelatedEntityType domain.RelatedEntityType
	RelatedEntityID   string
	AssigneeUserID    string
	Status            domain.ConversationStatus
	// Search matches contact name or phone.
	Search string
	Limit  int
	Offset int
}

// UpdateConversationParams applies only non-nil fields. An empty
// AssigneeUserID unassigns the conversation.
type UpdateConversationParams struct {
	AssigneeUserID *string
	Status         *domain.ConversationStatus
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

func (r *conversationRepository) GetByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Conversation, error) {
	if !scope.IsValid() {
		return domain.Conversation{}, coretenant.ErrInvalidScope
	}

	var conversation domain.Conversation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		conversation, scanErr = scanConversation(tx.QueryRow(ctx, `
			SELECT `+conversationColumns+` FROM wa_conversations WHERE id = $1 AND organization_id = $2
		`, id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Conversation{}, err
	}
	return conversation, nil
}

func (r *conversationRepository) List(ctx context.Context, scope coretenant.Scope, filter ConversationListFilter) ([]domain.Conversation, int64, error) {
	if !scope.IsValid() {
		return nil, 0, coretenant.ErrInvalidScope
	}

	where := []string{"organization_id = $1"}
	args := []any{scope.OrganizationID()}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if filter.RelatedEntityType != "" {
		add("related_entity_type = $%d", string(filter.RelatedEntityType))
	}
	if filter.RelatedEntityID != "" {
		add("related_entity_id = $%d::uuid", filter.RelatedEntityID)
	}
	if filter.AssigneeUserID != "" {
		add("assignee_user_id = $%d::uuid", filter.AssigneeUserID)
	}
	if filter.Status != "" {
		add("status = $%d", string(filter.Status))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		add("(contact_name ILIKE $%[1]d OR phone_normalized LIKE $%[1]d)", "%"+escapeLike(search)+"%")
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	whereSQL := strings.Join(where, " AND ")

	conversations := []domain.Conversation{}
	var total int64
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM wa_conversations WHERE `+whereSQL, args...).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+conversationColumns+`
			FROM wa_conversations
			WHERE `+whereSQL+`
			ORDER BY last_message_at DESC NULLS LAST, created_at DESC
			LIMIT `+fmt.Sprint(limit)+` OFFSET `+fmt.Sprint(offset), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			conversation, scanErr := scanConversation(rows)
			if scanErr != nil {
				return scanErr
			}
			conversations = append(conversations, conversation)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return conversations, total, nil
}

func (r *conversationRepository) LinkEntity(ctx context.Context, scope coretenant.Scope, id string, entityType domain.RelatedEntityType, entityID, assigneeUserID string) (domain.Conversation, error) {
	if !scope.IsValid() {
		return domain.Conversation{}, coretenant.ErrInvalidScope
	}

	var conversation domain.Conversation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE wa_conversations
			SET related_entity_type = $3, related_entity_id = $4,
				assignee_user_id = COALESCE(assignee_user_id, $5), updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND related_entity_id IS NULL
		`, id, scope.OrganizationID(), string(entityType), entityID, nullableString(assigneeUserID)); err != nil {
			return err
		}
		var scanErr error
		conversation, scanErr = scanConversation(tx.QueryRow(ctx, `
			SELECT `+conversationColumns+` FROM wa_conversations WHERE id = $1 AND organization_id = $2
		`, id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Conversation{}, err
	}
	return conversation, nil
}

func (r *conversationRepository) MarkRead(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}
	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE wa_conversations SET unread_count = 0, updated_at = now()
			WHERE id = $1 AND organization_id = $2
		`, id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *conversationRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateConversationParams) (domain.Conversation, error) {
	if !scope.IsValid() {
		return domain.Conversation{}, coretenant.ErrInvalidScope
	}

	sets := []string{"updated_at = now()"}
	args := []any{id, scope.OrganizationID()}
	if params.AssigneeUserID != nil {
		args = append(args, nullableString(*params.AssigneeUserID))
		sets = append(sets, fmt.Sprintf("assignee_user_id = $%d", len(args)))
	}
	if params.Status != nil {
		args = append(args, string(*params.Status))
		sets = append(sets, fmt.Sprintf("status = $%d", len(args)))
	}

	var conversation domain.Conversation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		conversation, scanErr = scanConversation(tx.QueryRow(ctx, `
			UPDATE wa_conversations SET `+strings.Join(sets, ", ")+`
			WHERE id = $1 AND organization_id = $2
			RETURNING `+conversationColumns, args...))
		return scanErr
	})
	if err != nil {
		return domain.Conversation{}, err
	}
	return conversation, nil
}

func (r *conversationRepository) ClaimActivityDay(ctx context.Context, scope coretenant.Scope, id string, day time.Time) (bool, error) {
	if !scope.IsValid() {
		return false, coretenant.ErrInvalidScope
	}
	claimed := false
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE wa_conversations SET crm_activity_on = $3::date
			WHERE id = $1 AND organization_id = $2 AND crm_activity_on IS DISTINCT FROM $3::date
		`, id, scope.OrganizationID(), day.Format("2006-01-02"))
		if err != nil {
			return err
		}
		claimed = tag.RowsAffected() == 1
		return nil
	})
	return claimed, err
}

func (r *conversationRepository) GetMessage(ctx context.Context, scope coretenant.Scope, id string) (domain.Message, error) {
	if !scope.IsValid() {
		return domain.Message{}, coretenant.ErrInvalidScope
	}
	var message domain.Message
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		message, scanErr = scanMessage(tx.QueryRow(ctx, `
			SELECT `+messageColumns+` FROM wa_messages WHERE id = $1 AND organization_id = $2
		`, id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Message{}, err
	}
	return message, nil
}

func (r *conversationRepository) ListMessages(ctx context.Context, scope coretenant.Scope, conversationID, beforeID string, limit int) ([]domain.Message, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	query := `SELECT ` + messageColumns + ` FROM wa_messages WHERE organization_id = $1 AND conversation_id = $2`
	args := []any{scope.OrganizationID(), conversationID}
	if beforeID != "" {
		// Keyset pagination on (sent_at, id); the cursor row must belong to
		// the same conversation.
		query += ` AND (sent_at, id) < (
			SELECT sent_at, id FROM wa_messages
			WHERE id = $3::uuid AND organization_id = $1 AND conversation_id = $2
		)`
		args = append(args, beforeID)
	}
	query += ` ORDER BY sent_at DESC, id DESC LIMIT ` + fmt.Sprint(limit)

	messages := []domain.Message{}
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			message, scanErr := scanMessage(rows)
			if scanErr != nil {
				return scanErr
			}
			messages = append(messages, message)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return messages, nil
}

func (r *conversationRepository) MarkMessageSent(ctx context.Context, scope coretenant.Scope, id, wahaMessageID string) (domain.Message, error) {
	if !scope.IsValid() {
		return domain.Message{}, coretenant.ErrInvalidScope
	}
	var message domain.Message
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			DELETE FROM wa_messages d
			USING wa_messages m
			WHERE m.id = $1 AND m.organization_id = $2
				AND d.organization_id = m.organization_id
				AND d.conversation_id = m.conversation_id
				AND d.waha_message_id = $3
				AND d.id <> m.id
		`, id, scope.OrganizationID(), wahaMessageID); err != nil {
			return err
		}
		var scanErr error
		message, scanErr = scanMessage(tx.QueryRow(ctx, `
			UPDATE wa_messages
			SET waha_message_id = $3, error = NULL,
				status = CASE WHEN status IN ('pending', 'failed') THEN 'sent' ELSE status END,
				updated_at = now()
			WHERE id = $1 AND organization_id = $2
			RETURNING `+messageColumns, id, scope.OrganizationID(), wahaMessageID))
		return scanErr
	})
	if err != nil {
		return domain.Message{}, err
	}
	return message, nil
}

func (r *conversationRepository) MarkMessageFailed(ctx context.Context, scope coretenant.Scope, id, errorMessage string) (domain.Message, error) {
	if !scope.IsValid() {
		return domain.Message{}, coretenant.ErrInvalidScope
	}
	var message domain.Message
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		message, scanErr = scanMessage(tx.QueryRow(ctx, `
			UPDATE wa_messages SET status = 'failed', error = $3, updated_at = now()
			WHERE id = $1 AND organization_id = $2
			RETURNING `+messageColumns, id, scope.OrganizationID(), errorMessage))
		return scanErr
	})
	if err != nil {
		return domain.Message{}, err
	}
	return message, nil
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
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
