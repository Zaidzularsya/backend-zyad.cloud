package cache

import (
	"testing"
	"time"
)

func TestInvalidationEventPayloadRoundTrip(t *testing.T) {
	occurredAt := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	event := NewInvalidationEvent(
		InvalidationKindEntitlement,
		cacheTestOrganizationID,
		"landing.enabled",
		3,
		"entitlement updated",
		occurredAt,
	)

	payload, err := event.MarshalJSONPayload()
	if err != nil {
		t.Fatalf("MarshalJSONPayload() error = %v", err)
	}
	parsed, err := ParseInvalidationEvent(payload)
	if err != nil {
		t.Fatalf("ParseInvalidationEvent() error = %v", err)
	}
	if parsed.Kind != InvalidationKindEntitlement ||
		parsed.OrganizationID != cacheTestOrganizationID ||
		parsed.ResourceID != "landing.enabled" ||
		parsed.Version != 3 ||
		!parsed.OccurredAt.Equal(occurredAt) {
		t.Fatalf("parsed event = %#v", parsed)
	}
}

func TestInvalidationEventRequiresTenant(t *testing.T) {
	event := NewInvalidationEvent(
		InvalidationKindDomain,
		"",
		"example.test",
		1,
		"",
		time.Now(),
	)

	if err := event.Validate(); err == nil {
		t.Fatal("Validate() expected error")
	}
}

func TestInvalidationKindRejectsUnknownValue(t *testing.T) {
	event := NewInvalidationEvent(
		InvalidationKind("unknown"),
		cacheTestOrganizationID,
		"",
		0,
		"",
		time.Now(),
	)

	if err := event.Validate(); err == nil {
		t.Fatal("Validate() expected error")
	}
}
