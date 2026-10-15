// Package realtime carries WhatsApp change notifications from the processes
// that cause them (worker webhook processor, API writes) to the API's SSE
// streams over Redis pub/sub. Events are hints only: they carry ids, never
// message bodies, and clients re-read state through the normal API. Delivery
// is at-most-once, so clients must refetch after every (re)connect.
package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type EventType string

const (
	// EventMessage: a message was stored (inbound, sent from the phone, or from the app).
	EventMessage EventType = "message"
	// EventAck: a message changed delivery status.
	EventAck EventType = "ack"
	// EventRead: a conversation was marked read (unread_count changed).
	EventRead EventType = "read"
	// EventConversation: conversation metadata changed (assignee/status).
	EventConversation EventType = "conversation"
)

const subscribeTimeout = 5 * time.Second

// Event identifies what changed. AssigneeUserID lets the stream apply the same
// "own vs read_all" visibility rule as the REST API without a database lookup.
type Event struct {
	Type              EventType `json:"type"`
	ConversationID    string    `json:"conversation_id"`
	MessageID         string    `json:"message_id,omitempty"`
	AssigneeUserID    string    `json:"assignee_user_id,omitempty"`
	RelatedEntityType string    `json:"related_entity_type,omitempty"`
	RelatedEntityID   string    `json:"related_entity_id,omitempty"`
}

// Publisher sends events. Implementations must not fail the caller: a missing
// notification only degrades clients to their polling fallback.
type Publisher interface {
	Publish(ctx context.Context, organizationID string, event Event)
}

// Subscriber streams one organization's events until the returned cancel is
// called or ctx ends, after which the channel is closed.
type Subscriber interface {
	Subscribe(ctx context.Context, organizationID string) (<-chan Event, func(), error)
}

type redisPubSub interface {
	Publish(ctx context.Context, channel string, message interface{}) *goredis.IntCmd
	Subscribe(ctx context.Context, channels ...string) *goredis.PubSub
}

// Bus is the Redis-backed Publisher and Subscriber.
type Bus struct {
	redis redisPubSub
	log   *slog.Logger
}

func NewBus(redis redisPubSub, log *slog.Logger) *Bus {
	if log == nil {
		log = slog.Default()
	}
	return &Bus{redis: redis, log: log}
}

func channel(organizationID string) string {
	return "wa:events:" + organizationID
}

func (b *Bus) Publish(ctx context.Context, organizationID string, event Event) {
	payload, err := json.Marshal(event)
	if err != nil {
		b.log.Warn("whatsapp realtime: marshal event", "error", err)
		return
	}
	if err := b.redis.Publish(ctx, channel(organizationID), payload).Err(); err != nil {
		b.log.Warn("whatsapp realtime: publish failed", "type", event.Type, "error", err)
	}
}

func (b *Bus) Subscribe(ctx context.Context, organizationID string) (<-chan Event, func(), error) {
	pubsub := b.redis.Subscribe(ctx, channel(organizationID))

	// Wait for the subscription to be active so an unreachable Redis is
	// reported now instead of leaving a stream that never delivers.
	confirmCtx, cancelConfirm := context.WithTimeout(ctx, subscribeTimeout)
	_, err := pubsub.Receive(confirmCtx)
	cancelConfirm()
	if err != nil {
		_ = pubsub.Close()
		return nil, nil, fmt.Errorf("subscribe: %w", err)
	}

	out := make(chan Event, 32)
	done := make(chan struct{})
	go func() {
		defer close(out)
		messages := pubsub.Channel()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case message, ok := <-messages:
				if !ok {
					return
				}
				var event Event
				if err := json.Unmarshal([]byte(message.Payload), &event); err != nil {
					b.log.Warn("whatsapp realtime: bad event payload", "error", err)
					continue
				}
				select {
				case out <- event:
				case <-done:
					return
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			close(done)
			_ = pubsub.Close()
		})
	}
	return out, cancel, nil
}
