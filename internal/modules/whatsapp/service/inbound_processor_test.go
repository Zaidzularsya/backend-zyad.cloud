package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/repository"
)

// --- fakes ---

type fakeEventStore struct {
	events    []domain.WebhookEvent
	processed map[string]bool
	failed    map[string]string
	retryAt   map[string]time.Time
}

func newFakeEventStore(events ...domain.WebhookEvent) *fakeEventStore {
	return &fakeEventStore{events: events, processed: map[string]bool{}, failed: map[string]string{}, retryAt: map[string]time.Time{}}
}

func (s *fakeEventStore) ClaimDue(context.Context, int, time.Duration) ([]domain.WebhookEvent, error) {
	return s.events, nil
}

func (s *fakeEventStore) MarkProcessed(_ context.Context, id string) error {
	s.processed[id] = true
	return nil
}

func (s *fakeEventStore) MarkFailed(_ context.Context, id string, msg string, next time.Time) error {
	s.failed[id] = msg
	s.retryAt[id] = next
	return nil
}

type fakeConversations struct {
	byChat       map[string]domain.Conversation
	messages     map[string]domain.Message // keyed by waha id (or a local key while pending)
	recorded     []repository.RecordMessageParams
	activityDays map[string]string
	seq          int
}

func newFakeConversations() *fakeConversations {
	return &fakeConversations{byChat: map[string]domain.Conversation{}, messages: map[string]domain.Message{}}
}

func (c *fakeConversations) GetBySessionChat(_ context.Context, _ coretenant.Scope, sessionID, chatID string) (domain.Conversation, error) {
	conversation, ok := c.byChat[sessionID+"|"+chatID]
	if !ok {
		return domain.Conversation{}, pgx.ErrNoRows
	}
	return conversation, nil
}

func (c *fakeConversations) Create(_ context.Context, scope coretenant.Scope, p repository.CreateConversationParams) (domain.Conversation, bool, error) {
	key := p.SessionID + "|" + p.ChatID
	if existing, ok := c.byChat[key]; ok {
		return existing, false, nil
	}
	conversation := domain.Conversation{
		ID: "conv-" + p.ChatID, OrganizationID: scope.OrganizationID(), SessionID: p.SessionID, ChatID: p.ChatID,
		PhoneNormalized: p.PhoneNormalized, ContactName: p.ContactName, RelatedEntityType: p.RelatedEntityType,
		RelatedEntityID: p.RelatedEntityID, AssigneeUserID: p.AssigneeUserID, Status: domain.ConversationStatusOpen,
	}
	c.byChat[key] = conversation
	return conversation, true, nil
}

func (c *fakeConversations) RecordMessage(_ context.Context, _ coretenant.Scope, conversationID string, p repository.RecordMessageParams) (domain.Message, bool, error) {
	key := p.WAHAMessageID
	if key != "" {
		if _, exists := c.messages[key]; exists {
			return domain.Message{}, false, nil
		}
	} else {
		c.seq++
		key = fmt.Sprintf("pending-%d", c.seq)
	}
	c.recorded = append(c.recorded, p)
	message := domain.Message{
		ID: "msg-" + key, ConversationID: conversationID, WAHAMessageID: p.WAHAMessageID, Direction: p.Direction,
		Body: p.Body, Status: p.Status, SentByUserID: p.SentByUserID, SentAt: p.SentAt,
	}
	c.messages[key] = message
	return message, true, nil
}

func (c *fakeConversations) FindMessageByWAHAID(_ context.Context, _ coretenant.Scope, _ string, wahaID string) (domain.Message, error) {
	message, ok := c.messages[wahaID]
	if !ok {
		return domain.Message{}, pgx.ErrNoRows
	}
	return message, nil
}

func (c *fakeConversations) SetMessageStatus(_ context.Context, _ coretenant.Scope, id string, from, to domain.MessageStatus) (bool, error) {
	for key, message := range c.messages {
		if message.ID == id && message.Status == from {
			message.Status = to
			c.messages[key] = message
			return true, nil
		}
	}
	return false, nil
}

type fakeCRM struct {
	matches    map[string]CRMMatch
	created    []CreateLeadInput
	activities []CRMActivityInput
}

func (m *fakeCRM) MatchPhone(_ context.Context, _ coretenant.Scope, phone string) (CRMMatch, bool, error) {
	match, ok := m.matches[phone]
	return match, ok, nil
}

func (m *fakeCRM) CreateLead(_ context.Context, _ coretenant.Scope, input CreateLeadInput) (CRMMatch, error) {
	m.created = append(m.created, input)
	return CRMMatch{EntityType: domain.RelatedEntityLead, EntityID: "lead-auto", Name: input.ContactName, OwnerUserID: input.OwnerUserID}, nil
}

func (m *fakeCRM) RecordActivity(_ context.Context, _ coretenant.Scope, input CRMActivityInput) error {
	m.activities = append(m.activities, input)
	return nil
}

type fakeLIDs map[string]string

func (l fakeLIDs) ResolveLID(_ context.Context, _ string, lid string) (string, error) {
	return l[lid], nil
}

type fakeNotifier struct {
	calls []domain.SessionStatus
}

func (n *fakeNotifier) NotifyDisconnected(_ context.Context, session domain.Session, _ domain.SessionStatus) error {
	n.calls = append(n.calls, session.Status)
	return nil
}

type fakeDirectoryResolver map[string]domain.DirectoryEntry

func (d fakeDirectoryResolver) Resolve(_ context.Context, name string) (domain.DirectoryEntry, error) {
	entry, ok := d[name]
	if !ok {
		return domain.DirectoryEntry{}, pgx.ErrNoRows
	}
	return entry, nil
}

// --- harness ---

type inboundHarness struct {
	processor     *InboundProcessor
	events        *fakeEventStore
	conversations *fakeConversations
	crm           *fakeCRM
	notifier      *fakeNotifier
	sessions      *fakeSessionRepo
	session       domain.Session
	scope         coretenant.Scope
}

func newInboundHarness(t *testing.T, autoCreateLead bool, events ...domain.WebhookEvent) *inboundHarness {
	t.Helper()
	sessions := newFakeSessionRepo()
	scope := testScope(t, testOrgID)
	session, _ := sessions.Create(context.Background(), scope, repository.CreateSessionParams{
		Name: "zc_test_abc123", Purpose: domain.SessionPurposeSales, AutoCreateLead: autoCreateLead,
	})
	session.CreatedBy = "creator-1"
	sessions.sessions[session.ID] = session

	notifier := &fakeNotifier{}
	svc := NewSessionService(sessions, &fakeProvider{}, testConfig, WithDisconnectNotifier(notifier))
	h := &inboundHarness{
		events:        newFakeEventStore(events...),
		conversations: newFakeConversations(),
		crm:           &fakeCRM{matches: map[string]CRMMatch{}},
		notifier:      notifier,
		sessions:      sessions,
		session:       session,
		scope:         scope,
	}
	h.processor = NewInboundProcessor(InboundProcessorDeps{
		Events:        h.events,
		Directory:     fakeDirectoryResolver{session.Name: {SessionName: session.Name, OrganizationID: testOrgID, SessionID: session.ID}},
		Resolver:      &fakeResolver{scopes: map[string]coretenant.Context{testOrgID: scopeContext(t, testOrgID)}},
		Sessions:      sessions,
		SessionSvc:    svc,
		Conversations: h.conversations,
		LIDs:          fakeLIDs{"999@lid": "6281111111111@c.us"},
		CRM:           h.crm,
	})
	return h
}

func webhookEvent(id, eventType, payload string) domain.WebhookEvent {
	return domain.WebhookEvent{
		ID: "row-" + id, EventID: id, SessionName: "zc_test_abc123", EventType: eventType, Attempts: 1,
		Payload: []byte(`{"id":"` + id + `","event":"` + eventType + `","session":"zc_test_abc123",` +
			`"me":{"id":"6289999999999@c.us","pushName":"Toko"},"payload":` + payload + `}`),
	}
}

func (h *inboundHarness) run(t *testing.T) ProcessResult {
	t.Helper()
	result, err := h.processor.RunOnce(context.Background(), 50)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	return result
}

// --- tests ---

func TestInboundMessageMatchesLead(t *testing.T) {
	h := newInboundHarness(t, false, webhookEvent("e1", "message.any",
		`{"id":"false_6281234567890@c.us_A1","timestamp":1790000000,"from":"6281234567890@c.us","to":"6289999999999@c.us","fromMe":false,"body":"Halo, mau tanya harga","hasMedia":false}`))
	h.crm.matches["6281234567890"] = CRMMatch{EntityType: domain.RelatedEntityLead, EntityID: "lead-1", Name: "Budi", OwnerUserID: "sales-1"}

	if result := h.run(t); result.Processed != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	conversation := h.conversations.byChat[h.session.ID+"|6281234567890@c.us"]
	if conversation.RelatedEntityID != "lead-1" || conversation.AssigneeUserID != "sales-1" || conversation.ContactName != "Budi" ||
		conversation.PhoneNormalized != "6281234567890" {
		t.Fatalf("conversation = %+v", conversation)
	}
	if len(h.conversations.recorded) != 1 {
		t.Fatalf("recorded = %d", len(h.conversations.recorded))
	}
	message := h.conversations.recorded[0]
	if message.Direction != domain.MessageDirectionIn || message.Body != "Halo, mau tanya harga" ||
		!message.SentAt.Equal(time.Unix(1790000000, 0).UTC()) || message.Raw["_data"] != nil {
		t.Fatalf("message = %+v", message)
	}
	if !h.events.processed["row-e1"] {
		t.Fatal("event not marked processed")
	}
}

func TestInboundMessageAutoCreatesLeadForSessionCreator(t *testing.T) {
	h := newInboundHarness(t, true, webhookEvent("e1", "message.any",
		`{"id":"m1","timestamp":1790000000,"from":"6285555555555@c.us","to":"x@c.us","fromMe":false,"body":"hi"}`))
	h.run(t)

	if len(h.crm.created) != 1 {
		t.Fatalf("created leads = %d, want 1", len(h.crm.created))
	}
	lead := h.crm.created[0]
	if lead.OwnerUserID != "creator-1" || lead.Phone != "+6285555555555" || lead.ContactName != "WhatsApp +6285555555555" {
		t.Fatalf("lead = %+v", lead)
	}
	conversation := h.conversations.byChat[h.session.ID+"|6285555555555@c.us"]
	if conversation.RelatedEntityID != "lead-auto" || conversation.AssigneeUserID != "creator-1" {
		t.Fatalf("conversation = %+v", conversation)
	}
}

func TestInboundMessageWithoutMatchAndAutoCreateOff(t *testing.T) {
	h := newInboundHarness(t, false, webhookEvent("e1", "message.any",
		`{"id":"m1","from":"6285555555555@c.us","fromMe":false,"body":"hi"}`))
	h.run(t)

	if len(h.crm.created) != 0 {
		t.Fatal("lead created although auto_create_lead is off")
	}
	conversation := h.conversations.byChat[h.session.ID+"|6285555555555@c.us"]
	if conversation.RelatedEntityID != "" || conversation.AssigneeUserID != "" || conversation.ContactName != "+6285555555555" {
		t.Fatalf("conversation = %+v", conversation)
	}
}

func TestInboundMessageOnlyMatchesNewConversation(t *testing.T) {
	h := newInboundHarness(t, true,
		webhookEvent("e1", "message.any", `{"id":"m1","from":"6285555555555@c.us","body":"1"}`),
		webhookEvent("e2", "message.any", `{"id":"m2","from":"6285555555555@c.us","body":"2"}`),
	)
	h.run(t)
	if len(h.crm.created) != 1 || len(h.conversations.recorded) != 2 {
		t.Fatalf("created leads = %d, messages = %d; want 1 and 2", len(h.crm.created), len(h.conversations.recorded))
	}
}

func TestInboundMessageFromPhoneIsOutbound(t *testing.T) {
	h := newInboundHarness(t, false, webhookEvent("e1", "message.any",
		`{"id":"true_6281234567890@c.us_B1","from":"6289999999999@c.us","to":"6281234567890@c.us","fromMe":true,"source":"app","body":"Siap","ack":2}`))
	h.run(t)

	if _, ok := h.conversations.byChat[h.session.ID+"|6281234567890@c.us"]; !ok {
		t.Fatal("conversation should be keyed by the recipient for fromMe messages")
	}
	message := h.conversations.recorded[0]
	if message.Direction != domain.MessageDirectionOut || message.Status != domain.MessageStatusDelivered || message.SentByUserID != "" {
		t.Fatalf("message = %+v", message)
	}
}

func TestInboundIgnoresGroupsBroadcastsAndPlainMessageEvent(t *testing.T) {
	h := newInboundHarness(t, false,
		webhookEvent("e1", "message.any", `{"id":"m1","from":"120363@g.us","body":"grup"}`),
		webhookEvent("e2", "message.any", `{"id":"m2","from":"status@broadcast","body":"status"}`),
		webhookEvent("e3", "message.any", `{"id":"m3","from":"1203@newsletter","body":"channel"}`),
		webhookEvent("e4", "message", `{"id":"m4","from":"6281234567890@c.us","body":"dup of message.any"}`),
	)
	if result := h.run(t); result.Processed != 4 {
		t.Fatalf("result = %+v", result)
	}
	if len(h.conversations.byChat) != 0 || len(h.conversations.recorded) != 0 {
		t.Fatalf("stored %d conversations / %d messages, want none", len(h.conversations.byChat), len(h.conversations.recorded))
	}
}

func TestInboundResolvesLID(t *testing.T) {
	h := newInboundHarness(t, false, webhookEvent("e1", "message.any", `{"id":"m1","from":"999@lid","body":"hi"}`))
	h.crm.matches["6281111111111"] = CRMMatch{EntityType: domain.RelatedEntityContact, EntityID: "contact-1", Name: "Sari", OwnerUserID: "sales-2"}
	h.run(t)

	conversation, ok := h.conversations.byChat[h.session.ID+"|6281111111111@c.us"]
	if !ok || conversation.RelatedEntityType != domain.RelatedEntityContact || conversation.AssigneeUserID != "sales-2" {
		t.Fatalf("conversation = %+v (found %v)", conversation, ok)
	}
}

func TestInboundDuplicateMessageIsIdempotent(t *testing.T) {
	payload := `{"id":"m1","from":"6281234567890@c.us","body":"hi"}`
	h := newInboundHarness(t, false, webhookEvent("e1", "message.any", payload), webhookEvent("e2", "message.any", payload))
	if result := h.run(t); result.Processed != 2 {
		t.Fatalf("result = %+v", result)
	}
	if len(h.conversations.recorded) != 1 {
		t.Fatalf("recorded = %d, want 1", len(h.conversations.recorded))
	}
}

func TestInboundAckMovesStatusForwardOnly(t *testing.T) {
	h := newInboundHarness(t, false,
		webhookEvent("e1", "message.ack", `{"id":"out-1","ack":3}`),
		webhookEvent("e2", "message.ack", `{"id":"out-1","ack":2}`),
		webhookEvent("e3", "message.ack", `{"id":"unknown","ack":3}`),
	)
	h.conversations.messages["out-1"] = domain.Message{ID: "msg-out-1", WAHAMessageID: "out-1", Direction: domain.MessageDirectionOut, Status: domain.MessageStatusSent}

	if result := h.run(t); result.Processed != 3 {
		t.Fatalf("result = %+v", result)
	}
	if status := h.conversations.messages["out-1"].Status; status != domain.MessageStatusRead {
		t.Fatalf("status = %q, want read (late delivered ack must not downgrade)", status)
	}
}

func TestInboundSessionStatusNotifiesOnDisconnectOnly(t *testing.T) {
	h := newInboundHarness(t, false,
		webhookEvent("e1", "session.status", `{"name":"zc_test_abc123","status":"WORKING"}`),
		webhookEvent("e2", "session.status", `{"name":"zc_test_abc123","status":"FAILED"}`),
		webhookEvent("e3", "session.status", `{"name":"zc_test_abc123","status":"STOPPED"}`),
	)
	h.run(t)

	stored, _ := h.sessions.GetByID(context.Background(), h.scope, h.session.ID)
	if stored.Status != domain.SessionStatusStopped || stored.Phone != "6289999999999" {
		t.Fatalf("session = %+v", stored)
	}
	if len(h.notifier.calls) != 1 || h.notifier.calls[0] != domain.SessionStatusFailed {
		t.Fatalf("notifications = %v, want exactly one for WORKING -> FAILED", h.notifier.calls)
	}
}

func TestUserActionsDoNotNotify(t *testing.T) {
	h := newInboundHarness(t, false, webhookEvent("e1", "session.status", `{"status":"WORKING"}`))
	h.run(t)

	svc := NewSessionService(h.sessions, &fakeProvider{}, testConfig, WithDisconnectNotifier(h.notifier))
	if _, err := svc.Stop(context.Background(), h.scope, h.session.ID); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if len(h.notifier.calls) != 0 {
		t.Fatalf("user stop notified owners: %v", h.notifier.calls)
	}
}

func TestInboundUnknownSessionAndMalformedPayload(t *testing.T) {
	unknown := webhookEvent("e1", "message.any", `{"id":"m1","from":"6281234567890@c.us"}`)
	unknown.SessionName = "zc_deleted_zzz999"
	malformed := domain.WebhookEvent{ID: "row-e2", EventID: "e2", SessionName: "zc_test_abc123", Payload: []byte(`not json`)}
	h := newInboundHarness(t, false, unknown, malformed)

	if result := h.run(t); result.Processed != 2 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if !h.events.processed["row-e1"] || !h.events.processed["row-e2"] {
		t.Fatalf("processed = %v", h.events.processed)
	}
}

type failingConversations struct{ *fakeConversations }

func (failingConversations) GetBySessionChat(context.Context, coretenant.Scope, string, string) (domain.Conversation, error) {
	return domain.Conversation{}, errors.New("db down")
}

func TestInboundFailureSchedulesRetryWithBackoff(t *testing.T) {
	event := webhookEvent("e1", "message.any", `{"id":"m1","from":"6281234567890@c.us"}`)
	event.Attempts = 3
	h := newInboundHarness(t, false, event)
	h.processor.conversations = failingConversations{h.conversations}
	now := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	h.processor.now = func() time.Time { return now }

	if result := h.run(t); result.Failed != 1 {
		t.Fatalf("result = %+v", result)
	}
	if h.events.failed["row-e1"] == "" || !h.events.retryAt["row-e1"].Equal(now.Add(90*time.Second)) {
		t.Fatalf("failed = %q retryAt = %v", h.events.failed["row-e1"], h.events.retryAt["row-e1"])
	}
	if retryBackoff(50) != maxRetryBackoff {
		t.Fatal("backoff is not capped")
	}
}

func TestInboundSkipsMessagesSentThroughAPI(t *testing.T) {
	h := newInboundHarness(t, false, webhookEvent("e1", "message.any",
		`{"id":"true_6281234567890@c.us_C1","from":"6289999999999@c.us","to":"6281234567890@c.us","fromMe":true,"source":"api","body":"dari API"}`))
	h.run(t)
	if len(h.conversations.recorded) != 0 {
		t.Fatal("API-sent message recorded by the processor; ConversationService.Send stores it")
	}
}

// gowsEvent builds a message.any event shaped like the GOWS engine sends it:
// "me" carries both the phone id and the Linked ID of the connected account.
func gowsEvent(id, payload string) domain.WebhookEvent {
	return domain.WebhookEvent{
		ID: "row-" + id, EventID: id, SessionName: "zc_test_abc123", EventType: "message.any", Attempts: 1,
		Payload: []byte(`{"id":"` + id + `","event":"message.any","session":"zc_test_abc123",` +
			`"me":{"id":"6289999999999@c.us","lid":"999000@lid","pushName":"CS"},"payload":` + payload + `}`),
	}
}

// Regression (dev, 2026-09-24): our own message in a group arrived with
// from=<group>, to=<our LID>, participant=<our LID>; the old code took "to",
// resolved our LID to our own number, and auto-created a lead for the CS
// number itself.
func TestInboundIgnoresOwnGroupMessageFromGOWS(t *testing.T) {
	h := newInboundHarness(t, true, gowsEvent("e1",
		`{"id":"g1","from":"120363237896492216@g.us","to":"999000@lid","participant":"999000@lid","fromMe":true,"source":"app","body":"di grup"}`))
	h.processor.lids = fakeLIDs{"999000@lid": "6289999999999@c.us"}
	h.run(t)

	if len(h.conversations.byChat) != 0 || len(h.crm.created) != 0 {
		t.Fatalf("conversations = %d, leads = %d; want none", len(h.conversations.byChat), len(h.crm.created))
	}
}

// Regression: GOWS sends our own direct message from the phone with the
// chat in "from" and "to" empty; it must be stored as outbound to the peer.
func TestInboundOwnDirectMessageFromGOWSUsesFromAsChat(t *testing.T) {
	h := newInboundHarness(t, false, gowsEvent("e1",
		`{"id":"d1","from":"116368@lid","to":"","fromMe":true,"source":"app","body":"halo kak"}`))
	h.processor.lids = fakeLIDs{"116368@lid": "6281234567890@c.us"}
	h.run(t)

	conversation, ok := h.conversations.byChat[h.session.ID+"|6281234567890@c.us"]
	if !ok {
		t.Fatalf("conversation with the peer not created: %v", h.conversations.byChat)
	}
	if len(h.conversations.recorded) != 1 || h.conversations.recorded[0].Direction != domain.MessageDirectionOut {
		t.Fatalf("recorded = %+v", h.conversations.recorded)
	}
	if conversation.PhoneNormalized != "6281234567890" {
		t.Fatalf("conversation = %+v", conversation)
	}
}

func TestInboundNeverChatsWithOwnNumber(t *testing.T) {
	h := newInboundHarness(t, true,
		// note to self
		gowsEvent("e1", `{"id":"s1","from":"6289999999999@c.us","to":"6289999999999@c.us","fromMe":true,"source":"app","body":"catatan"}`),
		// own number with a device suffix
		gowsEvent("e2", `{"id":"s2","from":"6289999999999:12@s.whatsapp.net","to":"","fromMe":true,"source":"app","body":"x"}`),
		// peer LID that resolves to our own number
		gowsEvent("e3", `{"id":"s3","from":"555@lid","fromMe":false,"body":"x"}`),
	)
	h.processor.lids = fakeLIDs{"555@lid": "6289999999999@c.us"}
	h.run(t)

	if len(h.conversations.byChat) != 0 || len(h.crm.created) != 0 {
		t.Fatalf("conversations = %v, leads = %d; want none", h.conversations.byChat, len(h.crm.created))
	}
}

func TestNormalizeJID(t *testing.T) {
	tests := map[string]string{
		"6281234567890@s.whatsapp.net":    "6281234567890@c.us",
		"6281234567890:12@s.whatsapp.net": "6281234567890@c.us",
		"168624744620063@lid":             "168624744620063@lid",
		" 628@c.us ":                      "628@c.us",
		"":                                "",
	}
	for input, want := range tests {
		if got := normalizeJID(input); got != want {
			t.Errorf("normalizeJID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestInboundWritesOneDailyActivityRegardlessOfMessageCountOrDirection(t *testing.T) {
	h := newInboundHarness(t, false,
		webhookEvent("e1", "message.any", `{"id":"m1","from":"6281234567890@c.us","fromMe":false,"body":"halo"}`),
		webhookEvent("e2", "message.any", `{"id":"m2","from":"6289999999999@c.us","to":"6281234567890@c.us","fromMe":true,"source":"app","body":"balas"}`),
		webhookEvent("e3", "message.any", `{"id":"m3","from":"6281234567890@c.us","fromMe":false,"body":"lagi"}`),
	)
	h.crm.matches["6281234567890"] = CRMMatch{EntityType: domain.RelatedEntityLead, EntityID: "lead-1", Name: "Budi", OwnerUserID: "sales-1"}
	h.run(t)

	if len(h.crm.activities) != 1 {
		t.Fatalf("activities = %+v, want exactly 1 for 3 messages the same day", h.crm.activities)
	}
	activity := h.crm.activities[0]
	if activity.EntityID != "lead-1" || activity.UserID != "sales-1" || !strings.Contains(activity.Subject, "6281234567890") {
		t.Fatalf("activity = %+v", activity)
	}
}

func TestInboundNoActivityForUnlinkedConversation(t *testing.T) {
	h := newInboundHarness(t, false, webhookEvent("e1", "message.any", `{"id":"m1","from":"6281234567890@c.us","fromMe":false,"body":"halo"}`))
	h.run(t)
	if len(h.crm.activities) != 0 {
		t.Fatalf("activities = %+v, want none (no CRM match)", h.crm.activities)
	}
}

func TestInboundDuplicateEventDoesNotReclaimActivity(t *testing.T) {
	payload := `{"id":"m1","from":"6281234567890@c.us","fromMe":false,"body":"halo"}`
	h := newInboundHarness(t, false, webhookEvent("e1", "message.any", payload), webhookEvent("e2", "message.any", payload))
	h.crm.matches["6281234567890"] = CRMMatch{EntityType: domain.RelatedEntityLead, EntityID: "lead-1", OwnerUserID: "sales-1"}
	h.run(t)
	if len(h.crm.activities) != 1 {
		t.Fatalf("activities = %+v, want 1 (second event is a duplicate message id)", h.crm.activities)
	}
}
