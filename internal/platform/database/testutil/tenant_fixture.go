package testutil

import (
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
)

type TenantFixture struct {
	OrganizationID string
	Context        coretenant.Context
	Scope          coretenant.Scope
}

type TenantPair struct {
	A TenantFixture
	B TenantFixture
}

func NewTenantPair(t testing.TB) TenantPair {
	t.Helper()
	return TenantPair{
		A: newTenantFixture(
			t,
			"11111111-1111-1111-1111-111111111111",
			"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			"organization-a",
		),
		B: newTenantFixture(
			t,
			"22222222-2222-2222-2222-222222222222",
			"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			"organization-b",
		),
	}
}

func newTenantFixture(
	t testing.TB,
	organizationID string,
	membershipID string,
	slug string,
) TenantFixture {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     organizationID,
		OrganizationSlug:   slug,
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID:       membershipID,
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("create tenant fixture context: %v", err)
	}
	scope, err := coretenant.NewScope(tenantContext)
	if err != nil {
		t.Fatalf("create tenant fixture scope: %v", err)
	}
	return TenantFixture{
		OrganizationID: organizationID,
		Context:        tenantContext,
		Scope:          scope,
	}
}
