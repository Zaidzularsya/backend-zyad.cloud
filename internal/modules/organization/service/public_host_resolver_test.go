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

type fakePublicHostResolverStore struct {
	resolved     repository.ResolvedDomain
	resolveErr   error
	organization model.Organization
	findErr      error
	resolvedHost string
}

func (f *fakePublicHostResolverStore) ResolveActiveHost(
	_ context.Context,
	host string,
) (repository.ResolvedDomain, error) {
	f.resolvedHost = host
	return f.resolved, f.resolveErr
}

func (f *fakePublicHostResolverStore) FindOrganizationByID(
	context.Context,
	string,
) (model.Organization, error) {
	return f.organization, f.findErr
}

func TestNormalizePublicHost(t *testing.T) {
	host, err := NormalizePublicHost(" WWW.Example.COM.:8443 ")
	if err != nil {
		t.Fatalf("NormalizePublicHost() error = %v", err)
	}
	if host != "www.example.com" {
		t.Fatalf("NormalizePublicHost() = %q", host)
	}

	for _, invalid := range []string{
		"https://example.com",
		"example.com/path",
		"example.com,evil.test",
		"example.com:99999",
		"[::1]:8080",
		"127.0.0.1",
		"localhost",
		"bad_host.example.com",
	} {
		if _, err := NormalizePublicHost(invalid); err == nil {
			t.Fatalf("NormalizePublicHost(%q) expected error", invalid)
		}
	}
}

func TestPublicHostResolverResolvesPlatformPrimaryDomain(t *testing.T) {
	store := &fakePublicHostResolverStore{
		organization: model.Organization{
			ID:            resolverOrganizationID,
			Slug:          "zyad-cloud",
			Type:          coretenant.OrganizationTypePlatform,
			Status:        coretenant.OrganizationStatusActive,
			DataPlacement: coretenant.DataPlacementShared,
		},
	}
	resolver := NewPublicHostResolver(store, resolverOrganizationID, "zyad.cloud")

	tenantContext, ok, err := resolver.ResolvePublicHost(
		context.Background(),
		"ZYAD.CLOUD.",
	)
	if err != nil {
		t.Fatalf("ResolvePublicHost() error = %v", err)
	}
	if !ok || tenantContext.ResolutionSource() != coretenant.ResolutionSourcePlatformHost {
		t.Fatalf("resolved context = %#v, %v", tenantContext, ok)
	}
}

func TestPublicHostResolverResolvesPlatformWWWAliasWithCanonicalHint(t *testing.T) {
	store := &fakePublicHostResolverStore{
		resolved: repository.ResolvedDomain{
			Domain: model.OrganizationDomain{
				Type:          model.DomainTypePlatform,
				CanonicalHost: "www.zyad.cloud",
				Status:        model.DomainStatusActive,
			},
			Organization: model.Organization{
				ID:            resolverOrganizationID,
				Slug:          "zyad-cloud",
				Type:          coretenant.OrganizationTypePlatform,
				Status:        coretenant.OrganizationStatusActive,
				DataPlacement: coretenant.DataPlacementShared,
			},
		},
	}
	resolver := NewPublicHostResolver(store, resolverOrganizationID, "zyad.cloud")

	resolution, err := resolver.ResolvePublicHostDetail(
		context.Background(),
		"www.zyad.cloud",
	)
	if err != nil {
		t.Fatalf("ResolvePublicHostDetail() error = %v", err)
	}
	if !resolution.Resolved ||
		resolution.TenantContext.ResolutionSource() != coretenant.ResolutionSourcePlatformHost ||
		!resolution.Redirect ||
		resolution.CanonicalHost != "zyad.cloud" {
		t.Fatalf("resolution = %#v", resolution)
	}
}

func TestPublicHostResolverResolvesVerifiedCustomDomain(t *testing.T) {
	store := &fakePublicHostResolverStore{
		resolved: repository.ResolvedDomain{
			Domain: model.OrganizationDomain{
				Type:   model.DomainTypeCustom,
				Status: model.DomainStatusActive,
			},
			Organization: model.Organization{
				ID:            resolverOrganizationID,
				Slug:          "acme",
				Type:          coretenant.OrganizationTypeCustomer,
				Status:        coretenant.OrganizationStatusActive,
				DataPlacement: coretenant.DataPlacementShared,
			},
		},
	}
	resolver := NewPublicHostResolver(store, "", "")

	tenantContext, ok, err := resolver.ResolvePublicHost(
		context.Background(),
		"landing.example.test",
	)
	if err != nil {
		t.Fatalf("ResolvePublicHost() error = %v", err)
	}
	if !ok || tenantContext.ResolutionSource() != coretenant.ResolutionSourceCustomDomain ||
		store.resolvedHost != "landing.example.test" {
		t.Fatalf("resolved context/store = %#v / %#v", tenantContext, store)
	}
}

func TestPublicHostResolverDoesNotResolveUnknownOrInactiveHost(t *testing.T) {
	store := &fakePublicHostResolverStore{resolveErr: pgx.ErrNoRows}
	resolver := NewPublicHostResolver(store, "", "")

	_, ok, err := resolver.ResolvePublicHost(
		context.Background(),
		"unknown.example.test",
	)
	if err != nil || ok {
		t.Fatalf("unknown resolution = %v, %v", ok, err)
	}

	store.resolveErr = nil
	store.resolved = repository.ResolvedDomain{
		Domain: model.OrganizationDomain{
			Type:   model.DomainTypeCustom,
			Status: model.DomainStatusDisabled,
		},
	}
	_, ok, err = resolver.ResolvePublicHost(
		context.Background(),
		"disabled.example.test",
	)
	if err != nil || ok {
		t.Fatalf("disabled resolution = %v, %v", ok, err)
	}
}

func TestPublicHostResolverRejectsInvalidHost(t *testing.T) {
	resolver := NewPublicHostResolver(&fakePublicHostResolverStore{}, "", "")
	_, _, err := resolver.ResolvePublicHost(context.Background(), "good.test,evil.test")
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "PUBLIC_HOST_INVALID" {
		t.Fatalf("ResolvePublicHost() error = %v", err)
	}
}
