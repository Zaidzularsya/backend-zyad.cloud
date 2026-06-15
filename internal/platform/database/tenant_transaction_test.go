package database

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
)

func TestSharedTenantPoolResolverRejectsDedicatedPlacement(t *testing.T) {
	resolver := NewSharedTenantPoolResolver(&Pool{})
	_, err := resolver.ResolveTenantPool(
		context.Background(),
		databaseTenantContext(t, coretenant.DataPlacementDedicated),
	)
	if !errors.Is(err, ErrDedicatedPlacementNotReady) {
		t.Fatalf("ResolveTenantPool() error = %v", err)
	}
}

func TestSharedTenantPoolResolverReturnsSharedPool(t *testing.T) {
	pool := &Pool{}
	resolver := NewSharedTenantPoolResolver(pool)
	resolved, err := resolver.ResolveTenantPool(
		context.Background(),
		databaseTenantContext(t, coretenant.DataPlacementShared),
	)
	if err != nil {
		t.Fatalf("ResolveTenantPool() error = %v", err)
	}
	if resolved != pool {
		t.Fatalf("ResolveTenantPool() = %#v, want %#v", resolved, pool)
	}
}

func TestTenantTransactorRejectsMissingContext(t *testing.T) {
	transactor := NewSharedTenantTransactor(&Pool{})
	_, err := transactor.Begin(context.Background())
	if !errors.Is(err, ErrTenantContextRequired) {
		t.Fatalf("Begin() error = %v", err)
	}
}

func TestTenantTransactorRejectsNilCallback(t *testing.T) {
	transactor := NewSharedTenantTransactor(&Pool{})
	err := transactor.Within(context.Background(), nil)
	if !errors.Is(err, ErrTenantTransactionCallback) {
		t.Fatalf("Within() error = %v", err)
	}
}

func databaseTenantContext(
	t *testing.T,
	placement coretenant.DataPlacement,
) coretenant.Context {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     "11111111-1111-1111-1111-111111111111",
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID:       "22222222-2222-2222-2222-222222222222",
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      placement,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	return tenantContext
}
