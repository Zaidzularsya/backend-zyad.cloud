package tenant

import (
	"context"
	"errors"
	"testing"
)

func TestRequireScopeUsesVerifiedContext(t *testing.T) {
	tenantContext := scopeTenantContext(t)
	scope, err := RequireScope(WithContext(context.Background(), tenantContext))
	if err != nil {
		t.Fatalf("RequireScope() error = %v", err)
	}
	if scope.OrganizationID() != tenantContext.OrganizationID() ||
		scope.DataPlacement() != tenantContext.DataPlacement() {
		t.Fatalf("RequireScope() = %#v", scope)
	}
}

func TestRequireScopeRejectsMissingContext(t *testing.T) {
	_, err := RequireScope(context.Background())
	if !errors.Is(err, ErrMissingContext) {
		t.Fatalf("RequireScope() error = %v", err)
	}
}

func TestScopeRejectsMixedOrganizations(t *testing.T) {
	scope, err := NewScope(scopeTenantContext(t))
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	if err := scope.ValidateOrganizations([]string{
		scope.OrganizationID(),
		"33333333-3333-3333-3333-333333333333",
	}); !errors.Is(err, ErrOrganizationScopeMismatch) {
		t.Fatalf("ValidateOrganizations() error = %v", err)
	}
}

func TestScopeAcceptsOrganizationBatch(t *testing.T) {
	scope, err := NewScope(scopeTenantContext(t))
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	if err := scope.ValidateOrganizations([]string{
		scope.OrganizationID(),
		scope.OrganizationID(),
	}); err != nil {
		t.Fatalf("ValidateOrganizations() error = %v", err)
	}
}

func scopeTenantContext(t *testing.T) Context {
	t.Helper()
	tenantContext, err := NewVerifiedContext(VerifiedContextInput{
		OrganizationID:     "11111111-1111-1111-1111-111111111111",
		OrganizationSlug:   "acme",
		OrganizationType:   OrganizationTypeCustomer,
		OrganizationStatus: OrganizationStatusActive,
		MembershipID:       "22222222-2222-2222-2222-222222222222",
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   ResolutionSourceSession,
		DataPlacement:      DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	return tenantContext
}
