package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	whatsappmodule "zyad.cloud/internal/modules/whatsapp"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
	"zyad.cloud/internal/shared/phone"
)

const maxMessageLength = 4096

// MessageSender is the subset of the WAHA client used to send messages.
type MessageSender interface {
	SendText(ctx context.Context, message platformwhatsapp.Message) (platformwhatsapp.Result, error)
	SendSeen(ctx context.Context, session, chatID string, messageIDs []string) error
}

// Viewer is the user calling the API. Without CanReadAll a user only sees
// conversations assigned to them (docs/reference-whatsapp.md).
type Viewer struct {
	UserID     string
	CanReadAll bool
	CanAssign  bool
}

func (v Viewer) canSee(conversation domain.Conversation) bool {
	return v.CanReadAll || (v.UserID != "" && conversation.AssigneeUserID == v.UserID)
}

type ConversationListInput struct {
	RelatedEntityType domain.RelatedEntityType
	RelatedEntityID   string
	AssigneeUserID    string
	Status            domain.ConversationStatus
	Search            string
	Limit             int
	Offset            int
}

type StartConversationInput struct {
	SessionID         string
	RelatedEntityType domain.RelatedEntityType
	RelatedEntityID   string
}

type UpdateConversationInput struct {
	AssigneeUserID *string
	Status         *domain.ConversationStatus
}

type MessagePage struct {
	// Messages are oldest first (chat order).
	Messages []domain.Message
	// NextBefore is the cursor for older messages; empty when none are left.
	NextBefore string
}

type ConversationService struct {
	conversations repository.ConversationRepository
	sessions      repository.SessionRepository
	sender        MessageSender
	crm           CRMEntities
	limiter       SendRateLimiter
	log           *slog.Logger
	now           func() time.Time
	location      *time.Location
}

type ConversationServiceDeps struct {
	Conversations repository.ConversationRepository
	Sessions      repository.SessionRepository
	// Sender may be nil when WAHA is not configured (sending then fails
	// with WHATSAPP_NOT_CONFIGURED).
	Sender  MessageSender
	CRM     CRMEntities
	Limiter SendRateLimiter
	Logger  *slog.Logger
}

func NewConversationService(deps ConversationServiceDeps) *ConversationService {
	log := deps.Logger
	if log == nil {
		log = slog.Default()
	}
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		location = time.FixedZone("WIB", 7*60*60)
	}
	return &ConversationService{
		conversations: deps.Conversations,
		sessions:      deps.Sessions,
		sender:        deps.Sender,
		crm:           deps.CRM,
		limiter:       deps.Limiter,
		log:           log,
		now:           func() time.Time { return time.Now().UTC() },
		location:      location,
	}
}

func (s *ConversationService) List(ctx context.Context, scope coretenant.Scope, viewer Viewer, input ConversationListInput) ([]domain.Conversation, int64, error) {
	filter := repository.ConversationListFilter{
		RelatedEntityType: input.RelatedEntityType,
		RelatedEntityID:   input.RelatedEntityID,
		AssigneeUserID:    input.AssigneeUserID,
		Status:            input.Status,
		Search:            input.Search,
		Limit:             input.Limit,
		Offset:            input.Offset,
	}
	if !viewer.CanReadAll {
		filter.AssigneeUserID = viewer.UserID
	}
	return s.conversations.List(ctx, scope, filter)
}

func (s *ConversationService) Get(ctx context.Context, scope coretenant.Scope, viewer Viewer, id string) (domain.Conversation, error) {
	conversation, err := s.conversations.GetByID(ctx, scope, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !viewer.canSee(conversation)) {
		return domain.Conversation{}, whatsappmodule.ErrConversationNotFound
	}
	return conversation, err
}

func (s *ConversationService) Messages(ctx context.Context, scope coretenant.Scope, viewer Viewer, id, before string, limit int) (MessagePage, error) {
	if _, err := s.Get(ctx, scope, viewer, id); err != nil {
		return MessagePage{}, err
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	newestFirst, err := s.conversations.ListMessages(ctx, scope, id, before, limit)
	if err != nil {
		return MessagePage{}, err
	}
	page := MessagePage{Messages: make([]domain.Message, len(newestFirst))}
	for i, message := range newestFirst {
		page.Messages[len(newestFirst)-1-i] = message
	}
	if len(newestFirst) == limit {
		page.NextBefore = newestFirst[len(newestFirst)-1].ID
	}
	return page, nil
}

func (s *ConversationService) MarkRead(ctx context.Context, scope coretenant.Scope, viewer Viewer, id string) error {
	if _, err := s.Get(ctx, scope, viewer, id); err != nil {
		return err
	}
	return s.conversations.MarkRead(ctx, scope, id)
}

// Update changes the assignee (needs CanAssign; the new assignee must be an
// active member) and/or the status.
func (s *ConversationService) Update(ctx context.Context, scope coretenant.Scope, viewer Viewer, id string, input UpdateConversationInput) (domain.Conversation, error) {
	conversation, err := s.Get(ctx, scope, viewer, id)
	if err != nil {
		return domain.Conversation{}, err
	}
	if input.Status != nil && *input.Status != domain.ConversationStatusOpen && *input.Status != domain.ConversationStatusClosed {
		return domain.Conversation{}, whatsappmodule.ErrInvalidStatus
	}
	if input.AssigneeUserID != nil && *input.AssigneeUserID != conversation.AssigneeUserID {
		if !viewer.CanAssign {
			return domain.Conversation{}, whatsappmodule.ErrAssignForbidden
		}
		if assignee := strings.TrimSpace(*input.AssigneeUserID); assignee != "" {
			active, err := s.crm.IsActiveMember(ctx, scope, assignee)
			if err != nil {
				return domain.Conversation{}, err
			}
			if !active {
				return domain.Conversation{}, whatsappmodule.ErrInvalidAssignee
			}
		}
	}
	return s.conversations.Update(ctx, scope, id, repository.UpdateConversationParams{
		AssigneeUserID: input.AssigneeUserID,
		Status:         input.Status,
	})
}

// Start opens (or reuses) the conversation with a lead's or contact's
// number. It is assigned to the entity owner, or to the viewer when the
// entity has no owner. A viewer without read_all cannot start a chat that
// belongs to someone else.
func (s *ConversationService) Start(ctx context.Context, scope coretenant.Scope, viewer Viewer, input StartConversationInput) (domain.Conversation, error) {
	if !input.RelatedEntityType.IsValid() || strings.TrimSpace(input.RelatedEntityID) == "" {
		return domain.Conversation{}, whatsappmodule.ErrInvalidEntityType
	}
	entity, err := s.crm.FindEntity(ctx, scope, input.RelatedEntityType, input.RelatedEntityID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Conversation{}, whatsappmodule.ErrEntityNotFound
	}
	if err != nil {
		return domain.Conversation{}, err
	}
	normalized, ok := phone.NormalizeID(entity.Phone)
	if !ok {
		return domain.Conversation{}, whatsappmodule.ErrEntityPhoneInvalid
	}

	session, err := s.sendingSession(ctx, scope, input.SessionID)
	if err != nil {
		return domain.Conversation{}, err
	}

	assignee := entity.OwnerUserID
	if assignee == "" {
		assignee = viewer.UserID
	}
	if !viewer.CanReadAll && assignee != viewer.UserID {
		return domain.Conversation{}, whatsappmodule.ErrConversationForbidden
	}

	conversation, created, err := s.conversations.Create(ctx, scope, repository.CreateConversationParams{
		SessionID:         session.ID,
		ChatID:            normalized + "@c.us",
		PhoneNormalized:   normalized,
		ContactName:       entity.Name,
		RelatedEntityType: entity.Type,
		RelatedEntityID:   entity.ID,
		AssigneeUserID:    assignee,
	})
	if err != nil {
		return domain.Conversation{}, err
	}
	if !created && conversation.RelatedEntityID == "" {
		// The contact wrote first (unknown number); link it now.
		conversation, err = s.conversations.LinkEntity(ctx, scope, conversation.ID, entity.Type, entity.ID, assignee)
		if err != nil {
			return domain.Conversation{}, err
		}
	}
	if !viewer.canSee(conversation) {
		return domain.Conversation{}, whatsappmodule.ErrConversationForbidden
	}
	if created {
		s.recordDailyActivity(ctx, scope, conversation, session, viewer.UserID)
	}
	return conversation, nil
}

// sendingSession returns the requested session or, when empty, the
// default connected one (falling back to any connected session).
func (s *ConversationService) sendingSession(ctx context.Context, scope coretenant.Scope, sessionID string) (domain.Session, error) {
	if sessionID != "" {
		session, err := s.sessions.GetByID(ctx, scope, sessionID)
		if err != nil {
			return domain.Session{}, whatsappmodule.MapSessionNotFound(err)
		}
		if session.Status != domain.SessionStatusWorking {
			return domain.Session{}, whatsappmodule.ErrSessionNotConnected
		}
		return session, nil
	}
	sessions, err := s.sessions.List(ctx, scope)
	if err != nil {
		return domain.Session{}, err
	}
	var fallback *domain.Session
	for i := range sessions {
		if sessions[i].Status != domain.SessionStatusWorking {
			continue
		}
		if sessions[i].IsDefault {
			return sessions[i], nil
		}
		if fallback == nil {
			fallback = &sessions[i]
		}
	}
	if fallback == nil {
		return domain.Session{}, whatsappmodule.ErrNoConnectedSession
	}
	return *fallback, nil
}

// Send stores the message as pending, sends it through WAHA, and records the
// result. A provider failure is not an API error: the message is returned
// with status failed so the UI can offer a retry.
func (s *ConversationService) Send(ctx context.Context, scope coretenant.Scope, viewer Viewer, conversationID, text string) (domain.Message, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxMessageLength {
		return domain.Message{}, whatsappmodule.ErrInvalidMessageText
	}
	conversation, err := s.Get(ctx, scope, viewer, conversationID)
	if err != nil {
		return domain.Message{}, err
	}
	session, err := s.readySession(ctx, scope, conversation)
	if err != nil {
		return domain.Message{}, err
	}

	message, _, err := s.conversations.RecordMessage(ctx, scope, conversation.ID, repository.RecordMessageParams{
		Direction:    domain.MessageDirectionOut,
		Body:         text,
		Preview:      text,
		Status:       domain.MessageStatusPending,
		SentByUserID: viewer.UserID,
		SentAt:       s.now(),
	})
	if err != nil {
		return domain.Message{}, err
	}
	return s.deliver(ctx, scope, conversation, session, message, viewer.UserID)
}

// Retry resends a failed outgoing message.
func (s *ConversationService) Retry(ctx context.Context, scope coretenant.Scope, viewer Viewer, messageID string) (domain.Message, error) {
	message, err := s.conversations.GetMessage(ctx, scope, messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Message{}, whatsappmodule.ErrMessageNotFound
	}
	if err != nil {
		return domain.Message{}, err
	}
	conversation, err := s.Get(ctx, scope, viewer, message.ConversationID)
	if err != nil {
		if errors.Is(err, whatsappmodule.ErrConversationNotFound) {
			return domain.Message{}, whatsappmodule.ErrMessageNotFound
		}
		return domain.Message{}, err
	}
	if message.Direction != domain.MessageDirectionOut || message.Status != domain.MessageStatusFailed {
		return domain.Message{}, whatsappmodule.ErrMessageNotRetryable
	}
	session, err := s.readySession(ctx, scope, conversation)
	if err != nil {
		return domain.Message{}, err
	}
	updated, err := s.conversations.SetMessageStatus(ctx, scope, message.ID, domain.MessageStatusFailed, domain.MessageStatusPending)
	if err != nil {
		return domain.Message{}, err
	}
	if !updated {
		return domain.Message{}, whatsappmodule.ErrMessageNotRetryable // retried concurrently
	}
	message.Status = domain.MessageStatusPending
	return s.deliver(ctx, scope, conversation, session, message, viewer.UserID)
}

func (s *ConversationService) readySession(ctx context.Context, scope coretenant.Scope, conversation domain.Conversation) (domain.Session, error) {
	if s.sender == nil {
		return domain.Session{}, whatsappmodule.ErrNotConfigured
	}
	session, err := s.sessions.GetByID(ctx, scope, conversation.SessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, whatsappmodule.ErrSessionNotConnected // session deleted
	}
	if err != nil {
		return domain.Session{}, err
	}
	if session.Status != domain.SessionStatusWorking {
		return domain.Session{}, whatsappmodule.ErrSessionNotConnected
	}
	if s.limiter != nil && !s.limiter.Allow(ctx, session.ID) {
		return domain.Session{}, whatsappmodule.ErrRateLimited
	}
	return session, nil
}

func (s *ConversationService) deliver(ctx context.Context, scope coretenant.Scope, conversation domain.Conversation, session domain.Session, message domain.Message, userID string) (domain.Message, error) {
	// WAHA recommends marking the chat as seen before replying (reduces the
	// risk of the number being flagged). Best effort.
	if conversation.UnreadCount > 0 {
		if err := s.sender.SendSeen(ctx, session.Name, conversation.ChatID, nil); err != nil {
			s.log.Info("whatsapp: send seen failed", "conversation_id", conversation.ID, "error", err)
		}
	}

	result, err := s.sender.SendText(ctx, platformwhatsapp.Message{
		Session: session.Name,
		To:      conversation.ChatID,
		Text:    message.Body,
	})
	if err != nil {
		s.log.Warn("whatsapp: send message failed", "conversation_id", conversation.ID, "message_id", message.ID, "error", err)
		failed, markErr := s.conversations.MarkMessageFailed(ctx, scope, message.ID, sendFailureReason(err))
		if markErr != nil {
			return domain.Message{}, markErr
		}
		return failed, nil
	}

	sent, err := s.conversations.MarkMessageSent(ctx, scope, message.ID, result.MessageID)
	if err != nil {
		return domain.Message{}, err
	}
	s.recordDailyActivity(ctx, scope, conversation, session, userID)
	return sent, nil
}

// recordDailyActivity writes at most one 'whatsapp' CRM activity per
// conversation per day (Asia/Jakarta). Failures are logged only: the chat
// itself already succeeded.
func (s *ConversationService) recordDailyActivity(ctx context.Context, scope coretenant.Scope, conversation domain.Conversation, session domain.Session, userID string) {
	claimAndRecordDailyActivity(ctx, scope, s.conversations, s.crm, s.now().In(s.location), conversation, session, userID, s.log)
}

// sendFailureReason is stored on the message and shown to users, so it
// never contains provider internals.
func sendFailureReason(err error) string {
	switch {
	case errors.Is(err, platformwhatsapp.ErrTimeout):
		return "WhatsApp provider timed out"
	case errors.Is(err, platformwhatsapp.ErrSessionNotFound):
		return "WhatsApp session no longer exists on the provider"
	default:
		return "WhatsApp provider rejected the message"
	}
}
