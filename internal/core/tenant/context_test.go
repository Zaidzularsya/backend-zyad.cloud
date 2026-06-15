package tenant

import (
	"context"
	"errors"
	"testing"
)

func TestNewVerifiedContext(t *testing.T) {
	tenantContext, err := NewVerifiedContext(VerifiedContextInput{
		OrganizationID:     "organization-1",
		OrganizationSlug:   "acme",
		OrganizationType:   OrganizationTypeCustomer,
		OrganizationStatus: OrganizationStatusActive,
		MembershipID:       "membership-1",
		MembershipStatus:   "active",
		MembershipVersion:  2,
		ResolutionSource:   ResolutionSourceSession,
		DataPlacement:      DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	if !tenantContext.IsActive() {
		t.Fatal("expected active tenant context")
	}
	if tenantContext.OrganizationID() != "organization-1" {
		t.Fatalf("OrganizationID() = %q", tenantContext.OrganizationID())
	}
}

func TestNewVerifiedContextRejectsAuthenticatedContextWithoutActiveMembership(t *testing.T) {
	_, err := NewVerifiedContext(VerifiedContextInput{
		OrganizationID:     "organization-1",
		OrganizationSlug:   "acme",
		OrganizationType:   OrganizationTypeCustomer,
		OrganizationStatus: OrganizationStatusActive,
		ResolutionSource:   ResolutionSourceHeader,
		DataPlacement:      DataPlacementShared,
	})
	if !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("NewVerifiedContext() error = %v, want ErrInvalidContext", err)
	}
}

func TestNewVerifiedContextRejectsIncompleteInput(t *testing.T) {
	_, err := NewVerifiedContext(VerifiedContextInput{})
	if !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("NewVerifiedContext() error = %v, want ErrInvalidContext", err)
	}
}

func TestContextRoundTrip(t *testing.T) {
	tenantContext, err := NewVerifiedContext(VerifiedContextInput{
		OrganizationID:     "organization-1",
		OrganizationSlug:   "zyad-cloud",
		OrganizationType:   OrganizationTypePlatform,
		OrganizationStatus: OrganizationStatusActive,
		ResolutionSource:   ResolutionSourcePlatformHost,
		DataPlacement:      DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}

	ctx := WithContext(context.Background(), tenantContext)
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("expected tenant context")
	}
	if got.OrganizationType() != OrganizationTypePlatform {
		t.Fatalf("OrganizationType() = %q", got.OrganizationType())
	}
}

func TestWithContextIgnoresInvalidContext(t *testing.T) {
	ctx := WithContext(context.Background(), Context{})
	if _, ok := FromContext(ctx); ok {
		t.Fatal("unexpected invalid tenant context")
	}
}

func TestRequireContextReturnsExplicitMissingError(t *testing.T) {
	_, err := RequireContext(context.Background())
	if !errors.Is(err, ErrMissingContext) {
		t.Fatalf("RequireContext() error = %v, want ErrMissingContext", err)
	}
}
