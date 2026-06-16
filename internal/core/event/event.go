package event

import (
	"errors"
	"net/http"
	"strings"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
)

var ErrInvalidEvent = errors.New("invalid tenant event")

type Envelope struct {
	ID             string
	Type           string
	OrganizationID string
	ActorUserID    string
	ResourceType   string
	ResourceID     string
	OccurredAt     time.Time
	Payload        map[string]any
}

type Input struct {
	ID           string
	Type         string
	ActorUserID  string
	ResourceType string
	ResourceID   string
	OccurredAt   time.Time
	Payload      map[string]any
}

func New(scope coretenant.Scope, input Input) (Envelope, error) {
	if !scope.IsValid() {
		return Envelope{}, ErrInvalidEvent
	}
	event := Envelope{
		ID:             strings.TrimSpace(input.ID),
		Type:           canonicalEventType(input.Type),
		OrganizationID: scope.OrganizationID(),
		ActorUserID:    strings.TrimSpace(input.ActorUserID),
		ResourceType:   canonicalEventType(input.ResourceType),
		ResourceID:     strings.TrimSpace(input.ResourceID),
		OccurredAt:     input.OccurredAt.UTC(),
		Payload:        clonePayload(input.Payload),
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	if err := event.Validate(); err != nil {
		return Envelope{}, err
	}
	return event, nil
}

func (e Envelope) Validate() error {
	if strings.TrimSpace(e.ID) == "" ||
		canonicalEventType(e.Type) == "" ||
		strings.TrimSpace(e.OrganizationID) == "" ||
		canonicalEventType(e.ResourceType) == "" ||
		strings.TrimSpace(e.ResourceID) == "" ||
		e.OccurredAt.IsZero() {
		return coreerrors.New(
			"TENANT_EVENT_INVALID",
			"tenant event envelope is invalid",
			http.StatusBadRequest,
		)
	}
	return nil
}

func (e Envelope) IdempotencyKey() (string, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"organization",
		e.OrganizationID,
		"event",
		e.Type,
		e.ID,
	}, ":"), nil
}

func (e Envelope) PayloadWithMetadata() map[string]any {
	payload := clonePayload(e.Payload)
	payload["event_id"] = e.ID
	payload["event_type"] = e.Type
	payload["organization_id"] = e.OrganizationID
	payload["resource_type"] = e.ResourceType
	payload["resource_id"] = e.ResourceID
	payload["occurred_at"] = e.OccurredAt.UTC().Format(time.RFC3339)
	if e.ActorUserID != "" {
		payload["actor_user_id"] = e.ActorUserID
	}
	return payload
}

func canonicalEventType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func clonePayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = value
	}
	return cloned
}
