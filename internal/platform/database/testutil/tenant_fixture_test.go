package testutil

import "testing"

func TestNewTenantPairCreatesDistinctVerifiedScopes(t *testing.T) {
	tenants := NewTenantPair(t)
	if tenants.A.OrganizationID == tenants.B.OrganizationID {
		t.Fatal("tenant fixture organizations must be distinct")
	}
	if tenants.A.Scope.OrganizationID() != tenants.A.OrganizationID {
		t.Fatalf("organization A scope = %q", tenants.A.Scope.OrganizationID())
	}
	if tenants.B.Scope.OrganizationID() != tenants.B.OrganizationID {
		t.Fatalf("organization B scope = %q", tenants.B.Scope.OrganizationID())
	}
	if !tenants.A.Context.IsActive() || !tenants.B.Context.IsActive() {
		t.Fatal("tenant fixture contexts must be active")
	}
}
