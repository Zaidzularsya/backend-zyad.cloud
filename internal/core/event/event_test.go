package event

import (
	"context"
	"errors"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
)

const (
	eventTestOrganizationID = "11111111-1111-1111-1111-111111111111"
	eventTestUserID         = "22222222-2222-2222-2222-222222222222"
)

func TestNewBuildsTenantEventFromScope(t *testing.T) {
	occurredAt := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	event, err := New(eventTestScope(t), Input{
		ID:           "event-1",
		Type:         "Organization.Created",
		ActorUserID:  eventTestUserID,
		ResourceType: "Organization",
		ResourceID:   eventTestOrganizationID,
		OccurredAt:   occurredAt,
		Payload:      map[string]any{"name": "Acme"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if event.OrganizationID != eventTestOrganizationID ||
		event.Type != "organization.created" ||
		event.ResourceType != "organization" {
		t.Fatalf("event = %#v", event)
	}
	key, err := event.IdempotencyKey()
	if err != nil {
		t.Fatalf("IdempotencyKey() error = %v", err)
	}
	want := "organization:" + eventTestOrganizationID +
		":event:organization.created:event-1"
	if key != want {
		t.Fatalf("IdempotencyKey() = %q, want %q", key, want)
	}
}

func TestNewRequiresScope(t *testing.T) {
	_, err := New(coretenant.Scope{}, Input{
		ID:           "event-1",
		Type:         "organization.created",
		ResourceType: "organization",
		ResourceID:   eventTestOrganizationID,
	})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("New() error = %v", err)
	}
}

func TestPayloadWithMetadataDoesNotMutateOriginalPayload(t *testing.T) {
	event, err := New(eventTestScope(t), Input{
		ID:           "event-1",
		Type:         "organization.created",
		ResourceType: "organization",
		ResourceID:   eventTestOrganizationID,
		Payload:      map[string]any{"name": "Acme"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	payload := event.PayloadWithMetadata()
	if payload["organization_id"] != eventTestOrganizationID ||
		payload["event_id"] != "event-1" {
		t.Fatalf("payload = %#v", payload)
	}
	if _, exists := event.Payload["organization_id"]; exists {
		t.Fatalf("original payload mutated: %#v", event.Payload)
	}
}

func TestPayloadWithMetadataDoesNotAllowTenantOverride(t *testing.T) {
	otherOrganizationID := "33333333-3333-3333-3333-333333333333"
	event, err := New(eventTestScope(t), Input{
		ID:           "event-1",
		Type:         "organization.created",
		ResourceType: "organization",
		ResourceID:   eventTestOrganizationID,
		Payload: map[string]any{
			"organization_id": otherOrganizationID,
			"name":            "Acme",
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	payload := event.PayloadWithMetadata()
	if payload["organization_id"] != eventTestOrganizationID {
		t.Fatalf("payload organization_id = %q, want %q", payload["organization_id"], eventTestOrganizationID)
	}
	if event.Payload["organization_id"] != otherOrganizationID {
		t.Fatalf("original payload organization_id = %#v", event.Payload["organization_id"])
	}
}

func TestWithWorkerTenantContextLoadsOrganization(t *testing.T) {
	event, err := New(eventTestScope(t), Input{
		ID:           "event-1",
		Type:         "organization.created",
		ResourceType: "organization",
		ResourceID:   eventTestOrganizationID,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	resolver := &eventTestWorkerResolver{
		tenantContext: eventTestTenantContext(t),
	}

	ctx, tenantContext, err := WithWorkerTenantContext(
		context.Background(),
		resolver,
		"notification-worker",
		event,
	)
	if err != nil {
		t.Fatalf("WithWorkerTenantContext() error = %v", err)
	}
	if resolver.organizationID != eventTestOrganizationID ||
		resolver.serviceIdentity != "notification-worker" ||
		tenantContext.OrganizationID() != eventTestOrganizationID {
		t.Fatalf("resolver=%#v tenant=%#v", resolver, tenantContext)
	}
	if fromContext, ok := coretenant.FromContext(ctx); !ok ||
		fromContext.OrganizationID() != eventTestOrganizationID {
		t.Fatalf("tenant context not attached")
	}
}

type eventTestWorkerResolver struct {
	organizationID  string
	serviceIdentity string
	tenantContext   coretenant.Context
}

func (r *eventTestWorkerResolver) ResolveWorkerOrganization(
	_ context.Context,
	organizationID string,
	serviceIdentity string,
) (coretenant.Context, error) {
	r.organizationID = organizationID
	r.serviceIdentity = serviceIdentity
	return r.tenantContext, nil
}

func eventTestScope(t *testing.T) coretenant.Scope {
	t.Helper()
	tenantContext := eventTestTenantContext(t)
	scope, err := coretenant.NewScope(tenantContext)
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	return scope
}

func eventTestTenantContext(t *testing.T) coretenant.Context {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     eventTestOrganizationID,
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceWorker,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	return tenantContext
}
