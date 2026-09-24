package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
	"zyad.cloud/internal/shared/phone"
)

const (
	webhookLease       = 2 * time.Minute
	maxRetryBackoff    = 10 * time.Minute
	baseRetryBackoff   = 10 * time.Second
	mediaPreview       = "[media]"
	autoLeadNamePrefix = "WhatsApp +"
)

type WebhookEventStore interface {
	ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]domain.WebhookEvent, error)
	MarkProcessed(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, errorMessage string, nextRetryAt time.Time) error
}

type DirectoryResolver interface {
	Resolve(ctx context.Context, sessionName string) (domain.DirectoryEntry, error)
}

// LIDResolver maps a WhatsApp Linked ID (xxx@lid) to its phone chat id.
type LIDResolver interface {
	ResolveLID(ctx context.Context, session, lid string) (string, error)
}

type ProcessResult struct {
	Claimed   int
	Processed int
	Failed    int
}

// InboundProcessor applies stored WAHA webhook events: session status,
// messages (inbound and sent from the phone), and delivery acks. It runs in
// the worker; the organization always comes from wa_session_directory.
type InboundProcessor struct {
	events        WebhookEventStore
	directory     DirectoryResolver
	resolver      WorkerTenantResolver
	sessions      repository.SessionRepository
	sessionSvc    *SessionService
	conversations repository.ConversationRepository
	lids          LIDResolver
	crm           CRMMatcher
	log           *slog.Logger
	now           func() time.Time
}

type InboundProcessorDeps struct {
	Events        WebhookEventStore
	Directory     DirectoryResolver
	Resolver      WorkerTenantResolver
	Sessions      repository.SessionRepository
	SessionSvc    *SessionService
	Conversations repository.ConversationRepository
	// LIDs may be nil; @lid chats then stay unresolved (no CRM match).
	LIDs   LIDResolver
	CRM    CRMMatcher
	Logger *slog.Logger
}

func NewInboundProcessor(deps InboundProcessorDeps) *InboundProcessor {
	log := deps.Logger
	if log == nil {
		log = slog.Default()
	}
	return &InboundProcessor{
		events:        deps.Events,
		directory:     deps.Directory,
		resolver:      deps.Resolver,
		sessions:      deps.Sessions,
		sessionSvc:    deps.SessionSvc,
		conversations: deps.Conversations,
		lids:          deps.LIDs,
		crm:           deps.CRM,
		log:           log,
		now:           func() time.Time { return time.Now().UTC() },
	}
}

// permanentError marks an event that can never succeed (malformed payload);
// it is marked processed instead of being retried.
type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

func (p *InboundProcessor) RunOnce(ctx context.Context, limit int) (ProcessResult, error) {
	events, err := p.events.ClaimDue(ctx, limit, webhookLease)
	if err != nil {
		return ProcessResult{}, err
	}

	result := ProcessResult{Claimed: len(events)}
	for _, event := range events {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		err := p.process(ctx, event)
		var permanent permanentError
		switch {
		case err == nil:
			result.Processed++
			if markErr := p.events.MarkProcessed(ctx, event.ID); markErr != nil {
				return result, markErr
			}
		case errors.As(err, &permanent):
			result.Processed++
			p.log.Warn("whatsapp: dropping malformed webhook event", "event_id", event.EventID, "type", event.EventType, "error", err)
			if markErr := p.events.MarkProcessed(ctx, event.ID); markErr != nil {
				return result, markErr
			}
		default:
			result.Failed++
			p.log.Warn("whatsapp: webhook event failed", "event_id", event.EventID, "type", event.EventType, "attempt", event.Attempts, "error", err)
			if markErr := p.events.MarkFailed(ctx, event.ID, err.Error(), p.now().Add(retryBackoff(event.Attempts))); markErr != nil {
				return result, markErr
			}
		}
	}
	return result, nil
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	backoff := time.Duration(attempt*attempt) * baseRetryBackoff
	if backoff > maxRetryBackoff {
		return maxRetryBackoff
	}
	return backoff
}

type webhookEnvelope struct {
	ID      string          `json:"id"`
	Event   string          `json:"event"`
	Session string          `json:"session"`
	Me      *webhookMe      `json:"me"`
	Payload json.RawMessage `json:"payload"`
}

type webhookMe struct {
	ID       string `json:"id"`
	PushName string `json:"pushName"`
}

type sessionStatusPayload struct {
	Status string `json:"status"`
}

type messagePayload struct {
	ID        string  `json:"id"`
	Timestamp float64 `json:"timestamp"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	FromMe    bool    `json:"fromMe"`
	Source    string  `json:"source"`
	Body      string  `json:"body"`
	HasMedia  bool    `json:"hasMedia"`
	Ack       *int    `json:"ack"`
}

type ackPayload struct {
	ID  string `json:"id"`
	Ack int    `json:"ack"`
}

func (p *InboundProcessor) process(ctx context.Context, event domain.WebhookEvent) error {
	var envelope webhookEnvelope
	if err := json.Unmarshal(event.Payload, &envelope); err != nil {
		return permanentError{fmt.Errorf("decode envelope: %w", err)}
	}
	switch envelope.Event {
	case domain.EventSessionStatus, domain.EventMessageAny, domain.EventMessageAck:
	default:
		// "message" duplicates "message.any"; other events are not used yet.
		return nil
	}

	entry, err := p.directory.Resolve(ctx, event.SessionName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // session deleted since the webhook arrived
	}
	if err != nil {
		return err
	}
	tenantContext, err := p.resolver.ResolveWorkerOrganization(ctx, entry.OrganizationID, WorkerIdentity)
	if err != nil {
		return err
	}
	scope, err := coretenant.NewScope(tenantContext)
	if err != nil {
		return err
	}
	ctx = coretenant.WithContext(ctx, tenantContext)

	session, err := p.sessions.GetByID(ctx, scope, entry.SessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	switch envelope.Event {
	case domain.EventSessionStatus:
		return p.handleSessionStatus(ctx, scope, session, envelope)
	case domain.EventMessageAny:
		return p.handleMessage(ctx, scope, session, envelope)
	default:
		return p.handleAck(ctx, scope, session, envelope)
	}
}

func (p *InboundProcessor) handleSessionStatus(ctx context.Context, scope coretenant.Scope, session domain.Session, envelope webhookEnvelope) error {
	var payload sessionStatusPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return permanentError{fmt.Errorf("decode session.status: %w", err)}
	}
	info := platformwhatsapp.SessionInfo{Name: session.Name, Status: payload.Status}
	if payload.Status == platformwhatsapp.SessionStatusWorking && envelope.Me != nil && envelope.Me.ID != "" {
		info.Me = &platformwhatsapp.Me{ID: envelope.Me.ID, PushName: envelope.Me.PushName}
	}
	_, err := p.sessionSvc.ApplyObservedStatus(ctx, scope, session, info)
	return err
}

func (p *InboundProcessor) handleMessage(ctx context.Context, scope coretenant.Scope, session domain.Session, envelope webhookEnvelope) error {
	var payload messagePayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return permanentError{fmt.Errorf("decode message.any: %w", err)}
	}
	if payload.ID == "" {
		return permanentError{errors.New("message without id")}
	}

	chatID := payload.From
	if payload.FromMe {
		chatID = payload.To
	}
	chatID = strings.Replace(strings.TrimSpace(chatID), "@s.whatsapp.net", "@c.us", 1)
	if chatID == "" || isIgnoredChat(chatID) {
		return nil
	}
	if strings.HasSuffix(chatID, "@lid") && p.lids != nil {
		pn, err := p.lids.ResolveLID(ctx, session.Name, chatID)
		if err != nil {
			return fmt.Errorf("resolve lid: %w", err)
		}
		if pn != "" {
			chatID = strings.Replace(pn, "@s.whatsapp.net", "@c.us", 1)
		}
	}

	conversation, err := p.conversationFor(ctx, scope, session, chatID)
	if err != nil {
		return err
	}

	direction := domain.MessageDirectionIn
	status := domain.MessageStatusDelivered
	if payload.FromMe {
		direction = domain.MessageDirectionOut
		status = domain.MessageStatusSent
		if payload.Ack != nil {
			if fromAck, ok := domain.MessageStatusFromAck(*payload.Ack); ok {
				status = fromAck
			}
		}
	}
	preview := payload.Body
	if preview == "" && payload.HasMedia {
		preview = mediaPreview
	}
	sentAt := p.now()
	if payload.Timestamp > 0 {
		sentAt = time.Unix(int64(payload.Timestamp), 0).UTC()
	}

	raw := map[string]any{
		"id":       payload.ID,
		"from":     payload.From,
		"to":       payload.To,
		"fromMe":   payload.FromMe,
		"source":   payload.Source,
		"hasMedia": payload.HasMedia,
	}
	if payload.Ack != nil {
		raw["ack"] = *payload.Ack
	}

	_, _, err = p.conversations.RecordMessage(ctx, scope, conversation.ID, repository.RecordMessageParams{
		WAHAMessageID: payload.ID,
		Direction:     direction,
		Body:          payload.Body,
		Preview:       preview,
		Status:        status,
		SentAt:        sentAt,
		Raw:           raw,
	})
	return err
}

// conversationFor returns the chat's conversation, creating it on the first
// message. Only a new conversation is matched to CRM (and may auto-create a
// lead), so a manual re-link later is never overwritten.
func (p *InboundProcessor) conversationFor(ctx context.Context, scope coretenant.Scope, session domain.Session, chatID string) (domain.Conversation, error) {
	conversation, err := p.conversations.GetBySessionChat(ctx, scope, session.ID, chatID)
	if err == nil {
		return conversation, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Conversation{}, err
	}

	params := repository.CreateConversationParams{SessionID: session.ID, ChatID: chatID}
	if strings.HasSuffix(chatID, "@c.us") {
		if normalized, ok := phone.NormalizeID(strings.TrimSuffix(chatID, "@c.us")); ok {
			params.PhoneNormalized = normalized
			params.ContactName = "+" + normalized
		}
	}

	if params.PhoneNormalized != "" && p.crm != nil {
		match, found, err := p.crm.MatchPhone(ctx, scope, params.PhoneNormalized)
		if err != nil {
			return domain.Conversation{}, fmt.Errorf("match crm: %w", err)
		}
		if !found && session.AutoCreateLead {
			match, err = p.crm.CreateLead(ctx, scope, CreateLeadInput{
				ContactName: autoLeadNamePrefix + params.PhoneNormalized,
				Phone:       "+" + params.PhoneNormalized,
				OwnerUserID: session.CreatedBy,
			})
			if err != nil {
				return domain.Conversation{}, fmt.Errorf("auto-create lead: %w", err)
			}
			found = true
		}
		if found {
			params.RelatedEntityType = match.EntityType
			params.RelatedEntityID = match.EntityID
			params.AssigneeUserID = match.OwnerUserID
			if match.Name != "" {
				params.ContactName = match.Name
			}
		}
	}

	conversation, _, err = p.conversations.Create(ctx, scope, params)
	return conversation, err
}

func (p *InboundProcessor) handleAck(ctx context.Context, scope coretenant.Scope, session domain.Session, envelope webhookEnvelope) error {
	var payload ackPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return permanentError{fmt.Errorf("decode message.ack: %w", err)}
	}
	next, ok := domain.MessageStatusFromAck(payload.Ack)
	if !ok || payload.ID == "" {
		return nil
	}

	message, err := p.conversations.FindMessageByWAHAID(ctx, scope, session.ID, payload.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // ack for a message we do not store (yet); a later ack updates it
	}
	if err != nil {
		return err
	}
	if !message.Status.CanTransitionTo(next) {
		return nil
	}
	_, err = p.conversations.SetMessageStatus(ctx, scope, message.ID, message.Status, next)
	return err
}

// isIgnoredChat skips groups, status updates, broadcast lists, and channels.
func isIgnoredChat(chatID string) bool {
	return strings.HasSuffix(chatID, "@g.us") ||
		strings.HasSuffix(chatID, "@broadcast") ||
		strings.HasSuffix(chatID, "@newsletter")
}
