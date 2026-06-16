package cache

import (
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
)

const cacheTestOrganizationID = "11111111-1111-1111-1111-111111111111"

func TestKeyBuilderTenantKeyRequiresScope(t *testing.T) {
	builder := NewKeyBuilder("")

	_, err := builder.TenantKey(coretenant.Scope{}, "landing")
	if !errors.Is(err, ErrInvalidKeyInput) {
		t.Fatalf("TenantKey() error = %v", err)
	}
}

func TestKeyBuilderTenantKeyCanonicalizesSegments(t *testing.T) {
	builder := NewKeyBuilder(" Zyad ")

	key, err := builder.TenantKey(
		cacheTestScope(t),
		"Landing Public",
		"WWW.Example.Test",
		"/home page",
	)
	if err != nil {
		t.Fatalf("TenantKey() error = %v", err)
	}
	want := "zyad:org:" + cacheTestOrganizationID +
		":landing_public:www.example.test:home_page"
	if key != want {
		t.Fatalf("TenantKey() = %q, want %q", key, want)
	}
}

func TestKeyBuilderVersionedKeys(t *testing.T) {
	builder := NewKeyBuilder("")
	scope := cacheTestScope(t)

	domainKey, err := builder.DomainKey(scope, "App.Example.Test", 7)
	if err != nil {
		t.Fatalf("DomainKey() error = %v", err)
	}
	permissionKey, err := builder.PermissionKey(scope, "user-1", "membership-1", 9)
	if err != nil {
		t.Fatalf("PermissionKey() error = %v", err)
	}
	entitlementKey, err := builder.EntitlementKey(scope, "Landing.Enabled", 11)
	if err != nil {
		t.Fatalf("EntitlementKey() error = %v", err)
	}
	landingKey, err := builder.LandingPublicKey(scope, "app.example.test", "Home", 13)
	if err != nil {
		t.Fatalf("LandingPublicKey() error = %v", err)
	}

	assertEqual(t, domainKey, "zyad:org:"+cacheTestOrganizationID+":domain:app.example.test:v7")
	assertEqual(
		t,
		permissionKey,
		"zyad:org:"+cacheTestOrganizationID+
			":permission:user:user-1:membership:membership-1:v9",
	)
	assertEqual(
		t,
		entitlementKey,
		"zyad:org:"+cacheTestOrganizationID+":entitlement:landing.enabled:v11",
	)
	assertEqual(
		t,
		landingKey,
		"zyad:org:"+cacheTestOrganizationID+
			":landing_public:app.example.test:home:page13",
	)
}

func TestKeyBuilderVersionedKeysRejectNonPositiveVersion(t *testing.T) {
	builder := NewKeyBuilder("")

	_, err := builder.EntitlementKey(cacheTestScope(t), "landing.enabled", 0)
	if !errors.Is(err, ErrInvalidKeyInput) {
		t.Fatalf("EntitlementKey() error = %v", err)
	}
}

func cacheTestScope(t *testing.T) coretenant.Scope {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     cacheTestOrganizationID,
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
		MembershipID:       "22222222-2222-2222-2222-222222222222",
		MembershipStatus:   "active",
		MembershipVersion:  1,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	scope, err := coretenant.NewScope(tenantContext)
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	return scope
}

func assertEqual(t *testing.T, got string, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
