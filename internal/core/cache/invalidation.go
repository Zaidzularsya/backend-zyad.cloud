package cache

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
)

const InvalidationTopic = "zyad.cache.invalidate.v1"

type InvalidationKind string

const (
	InvalidationKindMembership   InvalidationKind = "membership"
	InvalidationKindDomain       InvalidationKind = "domain"
	InvalidationKindEntitlement  InvalidationKind = "entitlement"
	InvalidationKindSession      InvalidationKind = "session"
	InvalidationKindOrganization InvalidationKind = "organization"
)

type InvalidationEvent struct {
	Kind           InvalidationKind `json:"kind"`
	OrganizationID string           `json:"organization_id"`
	ResourceID     string           `json:"resource_id,omitempty"`
	Version        int64            `json:"version,omitempty"`
	Reason         string           `json:"reason,omitempty"`
	OccurredAt     time.Time        `json:"occurred_at"`
}

func NewInvalidationEvent(
	kind InvalidationKind,
	organizationID string,
	resourceID string,
	version int64,
	reason string,
	occurredAt time.Time,
) InvalidationEvent {
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return InvalidationEvent{
		Kind:           kind,
		OrganizationID: strings.TrimSpace(organizationID),
		ResourceID:     strings.TrimSpace(resourceID),
		Version:        version,
		Reason:         strings.TrimSpace(reason),
		OccurredAt:     occurredAt.UTC(),
	}
}

func (k InvalidationKind) IsValid() bool {
	switch k {
	case InvalidationKindMembership,
		InvalidationKindDomain,
		InvalidationKindEntitlement,
		InvalidationKindSession,
		InvalidationKindOrganization:
		return true
	default:
		return false
	}
}

func (e InvalidationEvent) Validate() error {
	if !e.Kind.IsValid() ||
		strings.TrimSpace(e.OrganizationID) == "" ||
		e.OccurredAt.IsZero() {
		return coreerrors.New(
			"CACHE_INVALIDATION_EVENT_INVALID",
			"cache invalidation event is invalid",
			http.StatusBadRequest,
		)
	}
	if e.Version < 0 {
		return coreerrors.New(
			"CACHE_INVALIDATION_VERSION_INVALID",
			"cache invalidation version cannot be negative",
			http.StatusBadRequest,
		)
	}
	return nil
}

func (e InvalidationEvent) MarshalJSONPayload() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(e)
}

func ParseInvalidationEvent(payload []byte) (InvalidationEvent, error) {
	var event InvalidationEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return InvalidationEvent{}, err
	}
	if err := event.Validate(); err != nil {
		return InvalidationEvent{}, err
	}
	return event, nil
}
