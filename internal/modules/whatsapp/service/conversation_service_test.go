package service

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	whatsappmodule "zyad.cloud/internal/modules/whatsapp"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
	platformwhatsapp "zyad.cloud/internal/platform/whatsapp"
)

// --- fakeConversations: remaining ConversationRepository methods ---

func (c *fakeConversations) all() []domain.Conversation {
	out := []domain.Conversation{}
	for _, conversation := range c.byChat {
		out = append(out, conversation)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *fakeConversations) put(conversation domain.Conversation) {
	c.byChat[conversation.SessionID+"|"+conversation.ChatID] = conversation
}

func (c *fakeConversations) GetByID(_ context.Context, _ coretenant.Scope, id string) (domain.Conversation, error) {
	for _, conversation := range c.byChat {
		if conversation.ID == id {
			return conversation, nil
		}
	}
	return domain.Conversation{}, pgx.ErrNoRows
}

func (c *fakeConversations) List(_ context.Context, _ coretenant.Scope, filter repository.ConversationListFilter) ([]domain.Conversation, int64, error) {
	out := []domain.Conversation{}
	for _, conversation := range c.all() {
		if filter.AssigneeUserID != "" && conversation.AssigneeUserID != filter.AssigneeUserID {
			continue
		}
		if filter.RelatedEntityID != "" && conversation.RelatedEntityID != filter.RelatedEntityID {
			continue
		}
		out = append(out, conversation)
	}
	return out, int64(len(out)), nil
}

func (c *fakeConversations) LinkEntity(ctx context.Context, scope coretenant.Scope, id string, entityType domain.RelatedEntityType, entityID, assignee string) (domain.Conversation, error) {
	conversation, err := c.GetByID(ctx, scope, id)
	if err != nil {
		return domain.Conversation{}, err
	}
	if conversation.RelatedEntityID == "" {
		conversation.RelatedEntityType, conversation.RelatedEntityID = entityType, entityID
		if conversation.AssigneeUserID == "" {
			conversation.AssigneeUserID = assignee
		}
		c.put(conversation)
	}
	return conversation, nil
}

func (c *fakeConversations) MarkRead(ctx context.Context, scope coretenant.Scope, id string) error {
	conversation, err := c.GetByID(ctx, scope, id)
	if err != nil {
		return err
	}
	conversation.UnreadCount = 0
	c.put(conversation)
	return nil
}

func (c *fakeConversations) Update(ctx context.Context, scope coretenant.Scope, id string, p repository.UpdateConversationParams) (domain.Conversation, error) {
	conversation, err := c.GetByID(ctx, scope, id)
	if err != nil {
		return domain.Conversation{}, err
	}
	if p.AssigneeUserID != nil {
		conversation.AssigneeUserID = *p.AssigneeUserID
	}
	if p.Status != nil {
		conversation.Status = *p.Status
	}
	c.put(conversation)
	return conversation, nil
}

func (c *fakeConversations) ClaimActivityDay(_ context.Context, _ coretenant.Scope, id string, day time.Time) (bool, error) {
	if c.activityDays == nil {
		c.activityDays = map[string]string{}
	}
	key := day.Format("2006-01-02")
	if c.activityDays[id] == key {
		return false, nil
	}
	c.activityDays[id] = key
	return true, nil
}

func (c *fakeConversations) GetMessage(_ context.Context, _ coretenant.Scope, id string) (domain.Message, error) {
	for _, message := range c.messages {
		if message.ID == id {
			return message, nil
		}
	}
	return domain.Message{}, pgx.ErrNoRows
}

func (c *fakeConversations) ListMessages(_ context.Context, _ coretenant.Scope, conversationID, _ string, limit int) ([]domain.Message, error) {
	out := []domain.Message{}
	for _, message := range c.messages {
		if message.ConversationID == conversationID {
			out = append(out, message)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SentAt.After(out[j].SentAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (c *fakeConversations) keyOf(id string) (string, bool) {
	for key, message := range c.messages {
		if message.ID == id {
			return key, true
		}
	}
	return "", false
}

func (c *fakeConversations) MarkMessageSent(_ context.Context, _ coretenant.Scope, id, wahaID string) (domain.Message, error) {
	key, ok := c.keyOf(id)
	if !ok {
		return domain.Message{}, pgx.ErrNoRows
	}
	message := c.messages[key]
	delete(c.messages, key)
	message.WAHAMessageID, message.Status, message.Error = wahaID, domain.MessageStatusSent, ""
	c.messages[wahaID] = message
	return message, nil
}

func (c *fakeConversations) MarkMessageFailed(_ context.Context, _ coretenant.Scope, id, reason string) (domain.Message, error) {
	key, ok := c.keyOf(id)
	if !ok {
		return domain.Message{}, pgx.ErrNoRows
	}
	message := c.messages[key]
	message.Status, message.Error = domain.MessageStatusFailed, reason
	c.messages[key] = message
	return message, nil
}

// --- other fakes ---

type fakeSender struct {
	sent    []platformwhatsapp.Message
	seen    int
	sendErr error
}

func (s *fakeSender) SendText(_ context.Context, message platformwhatsapp.Message) (platformwhatsapp.Result, error) {
	if s.sendErr != nil {
		return platformwhatsapp.Result{}, s.sendErr
	}
	s.sent = append(s.sent, message)
	return platformwhatsapp.Result{MessageID: "true_waha_" + message.Text}, nil
}

func (s *fakeSender) SendSeen(context.Context, string, string, []string) error {
	s.seen++
	return nil
}

type fakeEntities struct {
	entities   map[string]CRMEntity
	members    map[string]bool
	activities []CRMActivityInput
}

func (e *fakeEntities) FindEntity(_ context.Context, _ coretenant.Scope, _ domain.RelatedEntityType, id string) (CRMEntity, error) {
	entity, ok := e.entities[id]
	if !ok {
		return CRMEntity{}, pgx.ErrNoRows
	}
	return entity, nil
}

func (e *fakeEntities) RecordActivity(_ context.Context, _ coretenant.Scope, input CRMActivityInput) error {
	e.activities = append(e.activities, input)
	return nil
}

func (e *fakeEntities) IsActiveMember(_ context.Context, _ coretenant.Scope, userID string) (bool, error) {
	return e.members[userID], nil
}

type fakeLimiter struct{ allow bool }

func (l fakeLimiter) Allow(context.Context, string) bool { return l.allow }

// --- harness ---

type conversationHarness struct {
	svc           *ConversationService
	conversations *fakeConversations
	sessions      *fakeSessionRepo
	sender        *fakeSender
	crm           *fakeEntities
	session       domain.Session
	scope         coretenant.Scope
}

func newConversationHarness(t *testing.T) *conversationHarness {
	t.Helper()
	scope := testScope(t, testOrgID)
	sessions := newFakeSessionRepo()
	session, _ := sessions.Create(context.Background(), scope, repository.CreateSessionParams{
		Name: "zc_test_abc123", DisplayName: "Sales", Purpose: domain.SessionPurposeSales, IsDefault: true,
	})
	session.Status = domain.SessionStatusWorking
	sessions.sessions[session.ID] = session

	h := &conversationHarness{
		conversations: newFakeConversations(),
		sessions:      sessions,
		sender:        &fakeSender{},
		crm: &fakeEntities{
			entities: map[string]CRMEntity{
				"lead-1":  {Type: domain.RelatedEntityLead, ID: "lead-1", Name: "Budi", Phone: "0812-3456-7890", OwnerUserID: "sales-1"},
				"lead-2":  {Type: domain.RelatedEntityLead, ID: "lead-2", Name: "Tanpa HP", Phone: "-"},
				"lead-3":  {Type: domain.RelatedEntityLead, ID: "lead-3", Name: "Tanpa owner", Phone: "081299990000"},
				"contact": {Type: domain.RelatedEntityContact, ID: "contact", Name: "Sari", Phone: "+6281277770000", OwnerUserID: "sales-2"},
			},
			members: map[string]bool{"sales-1": true, "sales-2": true},
		},
		session: session,
		scope:   scope,
	}
	h.svc = NewConversationService(ConversationServiceDeps{
		Conversations: h.conversations,
		Sessions:      sessions,
		Sender:        h.sender,
		CRM:           h.crm,
		Limiter:       fakeLimiter{allow: true},
	})
	h.svc.now = func() time.Time { return time.Date(2026, 9, 24, 3, 0, 0, 0, time.UTC) }
	return h
}

var (
	owner   = Viewer{UserID: "owner-1", CanReadAll: true, CanAssign: true}
	sales1  = Viewer{UserID: "sales-1"}
	sales2  = Viewer{UserID: "sales-2"}
	ctxTest = context.Background()
)

func (h *conversationHarness) start(t *testing.T, viewer Viewer, entityID string) domain.Conversation {
	t.Helper()
	entityType := domain.RelatedEntityLead
	if entityID == "contact" {
		entityType = domain.RelatedEntityContact
	}
	conversation, err := h.svc.Start(ctxTest, h.scope, viewer, StartConversationInput{RelatedEntityType: entityType, RelatedEntityID: entityID})
	if err != nil {
		t.Fatalf("Start(%s) error = %v", entityID, err)
	}
	return conversation
}

// --- tests ---

func TestStartConversationAssignsEntityOwner(t *testing.T) {
	h := newConversationHarness(t)
	conversation := h.start(t, sales1, "lead-1")

	if conversation.ChatID != "6281234567890@c.us" || conversation.AssigneeUserID != "sales-1" ||
		conversation.RelatedEntityID != "lead-1" || conversation.SessionID != h.session.ID || conversation.ContactName != "Budi" {
		t.Fatalf("conversation = %+v", conversation)
	}
	if len(h.crm.activities) != 1 || h.crm.activities[0].EntityID != "lead-1" || h.crm.activities[0].UserID != "sales-1" {
		t.Fatalf("activities = %+v", h.crm.activities)
	}
	// Starting again reuses the conversation and adds no activity.
	if again := h.start(t, sales1, "lead-1"); again.ID != conversation.ID || len(h.crm.activities) != 1 {
		t.Fatalf("second Start = %+v, activities = %d", again, len(h.crm.activities))
	}
}

func TestStartConversationRules(t *testing.T) {
	h := newConversationHarness(t)

	if _, err := h.svc.Start(ctxTest, h.scope, sales1, StartConversationInput{RelatedEntityType: domain.RelatedEntityLead, RelatedEntityID: "lead-2"}); !errors.Is(err, whatsappmodule.ErrEntityPhoneInvalid) {
		t.Fatalf("no phone error = %v", err)
	}
	if _, err := h.svc.Start(ctxTest, h.scope, sales1, StartConversationInput{RelatedEntityType: domain.RelatedEntityLead, RelatedEntityID: "missing"}); !errors.Is(err, whatsappmodule.ErrEntityNotFound) {
		t.Fatalf("missing entity error = %v", err)
	}
	if _, err := h.svc.Start(ctxTest, h.scope, sales1, StartConversationInput{RelatedEntityType: "deal", RelatedEntityID: "x"}); !errors.Is(err, whatsappmodule.ErrInvalidEntityType) {
		t.Fatalf("invalid type error = %v", err)
	}
	// sales-2 without read_all cannot open a chat owned by sales-1.
	if _, err := h.svc.Start(ctxTest, h.scope, sales2, StartConversationInput{RelatedEntityType: domain.RelatedEntityLead, RelatedEntityID: "lead-1"}); !errors.Is(err, whatsappmodule.ErrConversationForbidden) {
		t.Fatalf("foreign owner error = %v", err)
	}
	// Entity without owner: assigned to the viewer.
	if conversation := h.start(t, sales2, "lead-3"); conversation.AssigneeUserID != "sales-2" {
		t.Fatalf("unowned lead assignee = %q", conversation.AssigneeUserID)
	}
}

func TestStartConversationRequiresConnectedSession(t *testing.T) {
	h := newConversationHarness(t)
	session := h.sessions.sessions[h.session.ID]
	session.Status = domain.SessionStatusFailed
	h.sessions.sessions[h.session.ID] = session

	if _, err := h.svc.Start(ctxTest, h.scope, owner, StartConversationInput{RelatedEntityType: domain.RelatedEntityLead, RelatedEntityID: "lead-1"}); !errors.Is(err, whatsappmodule.ErrNoConnectedSession) {
		t.Fatalf("error = %v", err)
	}
	if _, err := h.svc.Start(ctxTest, h.scope, owner, StartConversationInput{SessionID: h.session.ID, RelatedEntityType: domain.RelatedEntityLead, RelatedEntityID: "lead-1"}); !errors.Is(err, whatsappmodule.ErrSessionNotConnected) {
		t.Fatalf("explicit session error = %v", err)
	}
}

func TestStartLinksExistingUnlinkedConversation(t *testing.T) {
	h := newConversationHarness(t)
	h.conversations.put(domain.Conversation{ID: "conv-x", SessionID: h.session.ID, ChatID: "6281234567890@c.us", PhoneNormalized: "6281234567890"})

	conversation := h.start(t, owner, "lead-1")
	if conversation.ID != "conv-x" || conversation.RelatedEntityID != "lead-1" || conversation.AssigneeUserID != "sales-1" {
		t.Fatalf("conversation = %+v", conversation)
	}
}

func TestVisibilityOwnVersusReadAll(t *testing.T) {
	h := newConversationHarness(t)
	mine := h.start(t, sales1, "lead-1")
	h.start(t, sales2, "contact")

	list, total, err := h.svc.List(ctxTest, h.scope, sales1, ConversationListInput{AssigneeUserID: "sales-2"})
	if err != nil || total != 1 || list[0].ID != mine.ID {
		t.Fatalf("sales-1 list = %+v (%d), %v; want only own", list, total, err)
	}
	if _, total, _ := h.svc.List(ctxTest, h.scope, owner, ConversationListInput{}); total != 2 {
		t.Fatalf("owner total = %d, want 2", total)
	}
	if _, err := h.svc.Get(ctxTest, h.scope, sales2, mine.ID); !errors.Is(err, whatsappmodule.ErrConversationNotFound) {
		t.Fatalf("sales-2 Get(sales-1) error = %v, want not found", err)
	}
	if _, err := h.svc.Send(ctxTest, h.scope, sales2, mine.ID, "hi"); !errors.Is(err, whatsappmodule.ErrConversationNotFound) {
		t.Fatalf("sales-2 Send(sales-1) error = %v, want not found", err)
	}
	if _, err := h.svc.Messages(ctxTest, h.scope, sales2, mine.ID, "", 10); !errors.Is(err, whatsappmodule.ErrConversationNotFound) {
		t.Fatalf("sales-2 Messages(sales-1) error = %v", err)
	}
}

func TestSendMessage(t *testing.T) {
	h := newConversationHarness(t)
	conversation := h.start(t, sales1, "lead-1")
	conversation.UnreadCount = 2
	h.conversations.put(conversation)

	message, err := h.svc.Send(ctxTest, h.scope, sales1, conversation.ID, "  Halo Budi  ")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if message.Status != domain.MessageStatusSent || message.WAHAMessageID != "true_waha_Halo Budi" || message.SentByUserID != "sales-1" {
		t.Fatalf("message = %+v", message)
	}
	sent := h.sender.sent[0]
	if sent.Session != h.session.Name || sent.To != "6281234567890@c.us" || sent.Text != "Halo Budi" {
		t.Fatalf("sent = %+v", sent)
	}
	if h.sender.seen != 1 {
		t.Fatalf("send seen calls = %d, want 1 (unread inbound)", h.sender.seen)
	}
	// Same day: the Start activity already covers today.
	if len(h.crm.activities) != 1 {
		t.Fatalf("activities = %d, want 1 per day", len(h.crm.activities))
	}
}

func TestSendMessageValidationAndLimits(t *testing.T) {
	h := newConversationHarness(t)
	conversation := h.start(t, sales1, "lead-1")

	if _, err := h.svc.Send(ctxTest, h.scope, sales1, conversation.ID, "   "); !errors.Is(err, whatsappmodule.ErrInvalidMessageText) {
		t.Fatalf("empty text error = %v", err)
	}
	h.svc.limiter = fakeLimiter{allow: false}
	if _, err := h.svc.Send(ctxTest, h.scope, sales1, conversation.ID, "hi"); !errors.Is(err, whatsappmodule.ErrRateLimited) {
		t.Fatalf("rate limit error = %v", err)
	}
	h.svc.limiter = fakeLimiter{allow: true}

	session := h.sessions.sessions[h.session.ID]
	session.Status = domain.SessionStatusScanQRCode
	h.sessions.sessions[h.session.ID] = session
	if _, err := h.svc.Send(ctxTest, h.scope, sales1, conversation.ID, "hi"); !errors.Is(err, whatsappmodule.ErrSessionNotConnected) {
		t.Fatalf("disconnected error = %v", err)
	}
	if len(h.sender.sent) != 0 {
		t.Fatal("message sent despite validation failures")
	}
}

func TestSendFailureIsStoredAndRetryable(t *testing.T) {
	h := newConversationHarness(t)
	conversation := h.start(t, sales1, "lead-1")
	h.sender.sendErr = platformwhatsapp.ErrTimeout

	failed, err := h.svc.Send(ctxTest, h.scope, sales1, conversation.ID, "Halo")
	if err != nil {
		t.Fatalf("Send() error = %v, want failed message instead", err)
	}
	if failed.Status != domain.MessageStatusFailed || failed.Error != "WhatsApp provider timed out" {
		t.Fatalf("failed = %+v", failed)
	}

	h.sender.sendErr = nil
	retried, err := h.svc.Retry(ctxTest, h.scope, sales1, failed.ID)
	if err != nil || retried.Status != domain.MessageStatusSent || retried.Error != "" {
		t.Fatalf("Retry() = %+v, %v", retried, err)
	}
	if _, err := h.svc.Retry(ctxTest, h.scope, sales1, retried.ID); !errors.Is(err, whatsappmodule.ErrMessageNotRetryable) {
		t.Fatalf("retry of sent message error = %v", err)
	}
	if _, err := h.svc.Retry(ctxTest, h.scope, sales2, failed.ID); !errors.Is(err, whatsappmodule.ErrMessageNotFound) {
		t.Fatalf("foreign retry error = %v, want not found", err)
	}
}

func TestUpdateConversationAssignment(t *testing.T) {
	h := newConversationHarness(t)
	conversation := h.start(t, sales1, "lead-1")
	toSales2, stranger := "sales-2", "stranger"
	closed := domain.ConversationStatusClosed

	if _, err := h.svc.Update(ctxTest, h.scope, sales1, conversation.ID, UpdateConversationInput{AssigneeUserID: &toSales2}); !errors.Is(err, whatsappmodule.ErrAssignForbidden) {
		t.Fatalf("assign without permission error = %v", err)
	}
	if updated, err := h.svc.Update(ctxTest, h.scope, sales1, conversation.ID, UpdateConversationInput{Status: &closed}); err != nil || updated.Status != closed {
		t.Fatalf("close by assignee = %+v, %v", updated, err)
	}
	if _, err := h.svc.Update(ctxTest, h.scope, owner, conversation.ID, UpdateConversationInput{AssigneeUserID: &stranger}); !errors.Is(err, whatsappmodule.ErrInvalidAssignee) {
		t.Fatalf("non-member assignee error = %v", err)
	}
	if updated, err := h.svc.Update(ctxTest, h.scope, owner, conversation.ID, UpdateConversationInput{AssigneeUserID: &toSales2}); err != nil || updated.AssigneeUserID != "sales-2" {
		t.Fatalf("reassign = %+v, %v", updated, err)
	}
}

func TestMessagesPageIsChronological(t *testing.T) {
	h := newConversationHarness(t)
	conversation := h.start(t, sales1, "lead-1")
	base := time.Date(2026, 9, 24, 1, 0, 0, 0, time.UTC)
	for i, text := range []string{"a", "b", "c"} {
		h.conversations.messages["w"+text] = domain.Message{ID: "m" + text, ConversationID: conversation.ID, Body: text, SentAt: base.Add(time.Duration(i) * time.Minute)}
	}

	page, err := h.svc.Messages(ctxTest, h.scope, sales1, conversation.ID, "", 2)
	if err != nil {
		t.Fatalf("Messages() error = %v", err)
	}
	if len(page.Messages) != 2 || page.Messages[0].Body != "b" || page.Messages[1].Body != "c" || page.NextBefore != "mb" {
		t.Fatalf("page = %+v", page)
	}
}
