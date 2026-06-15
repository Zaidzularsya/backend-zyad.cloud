package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

const (
	resolverUserID         = "11111111-1111-1111-1111-111111111111"
	resolverSessionID      = "22222222-2222-2222-2222-222222222222"
	resolverOrganizationID = "33333333-3333-3333-3333-333333333333"
	resolverMembershipID   = "44444444-4444-4444-4444-444444444444"
)

type fakeAuthenticatedResolverStore struct {
	snapshot        repository.SessionOrganizationSnapshot
	snapshotErr     error
	result          repository.MembershipOrganization
	findErr         error
	sessionFindErr  error
	listResults     []repository.MembershipOrganization
	listErr         error
	selectedOrgID   string
	sessionSnapshot repository.SessionOrganizationSnapshot
}

func (f *fakeAuthenticatedResolverStore) FindSessionSnapshot(
	context.Context,
	string,
	string,
) (repository.SessionOrganizationSnapshot, error) {
	return f.snapshot, f.snapshotErr
}

func (f *fakeAuthenticatedResolverStore) FindActiveMembershipOrganization(
	_ context.Context,
	_ string,
	organizationID string,
) (repository.MembershipOrganization, error) {
	f.selectedOrgID = organizationID
	return f.result, f.findErr
}

func (f *fakeAuthenticatedResolverStore) FindSessionMembershipOrganization(
	_ context.Context,
	_ string,
	snapshot repository.SessionOrganizationSnapshot,
) (repository.MembershipOrganization, error) {
	f.sessionSnapshot = snapshot
	return f.result, f.sessionFindErr
}

func (f *fakeAuthenticatedResolverStore) ListActiveMembershipOrganizations(
	context.Context,
	string,
	int,
) ([]repository.MembershipOrganization, error) {
	return f.listResults, f.listErr
}

func TestAuthenticatedResolverUsesVerifiedHeaderMembership(t *testing.T) {
	store := &fakeAuthenticatedResolverStore{result: resolverMembershipOrganization()}
	resolver := NewAuthenticatedResolver(store)

	tenantContext, ok, err := resolver.ResolveAuthenticatedOrganization(
		context.Background(),
		resolverUserID,
		resolverSessionID,
		resolverOrganizationID,
		"app.example.com",
	)
	if err != nil {
		t.Fatalf("ResolveAuthenticatedOrganization() error = %v", err)
	}
	if !ok || tenantContext.ResolutionSource() != coretenant.ResolutionSourceHeader ||
		tenantContext.MembershipVersion() != 3 ||
		store.selectedOrgID != resolverOrganizationID {
		t.Fatalf("resolved context/store = %#v / %#v", tenantContext, store)
	}
}

func TestAuthenticatedResolverRejectsForgedHeader(t *testing.T) {
	store := &fakeAuthenticatedResolverStore{findErr: pgx.ErrNoRows}
	resolver := NewAuthenticatedResolver(store)

	_, _, err := resolver.ResolveAuthenticatedOrganization(
		context.Background(),
		resolverUserID,
		resolverSessionID,
		resolverOrganizationID,
		"",
	)
	assertResolverErrorCode(t, err, "ORGANIZATION_ACCESS_DENIED")
}

func TestAuthenticatedResolverRejectsStaleSessionSnapshot(t *testing.T) {
	store := &fakeAuthenticatedResolverStore{
		snapshot: repository.SessionOrganizationSnapshot{
			OrganizationID:    resolverOrganizationID,
			MembershipID:      resolverMembershipID,
			MembershipVersion: 2,
		},
		sessionFindErr: pgx.ErrNoRows,
	}
	resolver := NewAuthenticatedResolver(store)

	_, _, err := resolver.ResolveAuthenticatedOrganization(
		context.Background(),
		resolverUserID,
		resolverSessionID,
		"",
		"",
	)
	assertResolverErrorCode(t, err, "ORGANIZATION_CONTEXT_STALE")
}

func TestAuthenticatedResolverDefaultsOnlySingleMembership(t *testing.T) {
	result := resolverMembershipOrganization()
	store := &fakeAuthenticatedResolverStore{
		listResults: []repository.MembershipOrganization{result},
	}
	resolver := NewAuthenticatedResolver(store)

	tenantContext, ok, err := resolver.ResolveAuthenticatedOrganization(
		context.Background(),
		resolverUserID,
		resolverSessionID,
		"",
		"",
	)
	if err != nil {
		t.Fatalf("ResolveAuthenticatedOrganization() error = %v", err)
	}
	if !ok || tenantContext.ResolutionSource() != coretenant.ResolutionSourceMembership {
		t.Fatalf("resolved context = %#v, %v", tenantContext, ok)
	}

	store.listResults = append(store.listResults, result)
	_, ok, err = resolver.ResolveAuthenticatedOrganization(
		context.Background(),
		resolverUserID,
		resolverSessionID,
		"",
		"",
	)
	if err != nil || ok {
		t.Fatalf("multiple membership resolution = %v, %v", ok, err)
	}
}

func resolverMembershipOrganization() repository.MembershipOrganization {
	return repository.MembershipOrganization{
		Membership: model.Membership{
			ID:             resolverMembershipID,
			OrganizationID: resolverOrganizationID,
			UserID:         resolverUserID,
			Status:         model.MembershipStatusActive,
			Version:        3,
		},
		Organization: model.Organization{
			ID:            resolverOrganizationID,
			Slug:          "acme",
			Type:          coretenant.OrganizationTypeCustomer,
			Status:        coretenant.OrganizationStatusActive,
			DataPlacement: coretenant.DataPlacementShared,
		},
	}
}

func assertResolverErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}
