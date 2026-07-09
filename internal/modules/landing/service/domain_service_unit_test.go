package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	landingdomain "zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

type landingDomainRepoStub struct {
	availableDomains []landingdomain.AvailableDomain
	allBindings      []landingdomain.DomainBinding
	bindCalled       bool
}

func (s *landingDomainRepoStub) BindDomain(
	context.Context,
	coretenant.Scope,
	repository.BindDomainParams,
) (landingdomain.DomainBinding, error) {
	s.bindCalled = true
	return landingdomain.DomainBinding{ID: "binding-1"}, nil
}

func (s *landingDomainRepoStub) UnbindDomain(context.Context, coretenant.Scope, string) error {
	return nil
}

func (s *landingDomainRepoStub) SetPrimaryBinding(context.Context, coretenant.Scope, string, string) error {
	return nil
}

func (s *landingDomainRepoStub) ListBindings(
	context.Context,
	coretenant.Scope,
	string,
) ([]landingdomain.DomainBinding, error) {
	return nil, nil
}

func (s *landingDomainRepoStub) ListAllBindings(
	context.Context,
	coretenant.Scope,
) ([]landingdomain.DomainBinding, error) {
	return s.allBindings, nil
}

func (s *landingDomainRepoStub) ListAvailableDomains(
	context.Context,
	coretenant.Scope,
) ([]landingdomain.AvailableDomain, error) {
	return s.availableDomains, nil
}

type landingDomainPageRepoStub struct{}

func (s *landingDomainPageRepoStub) Create(
	context.Context,
	coretenant.Scope,
	repository.CreatePageParams,
) (landingdomain.LandingPage, error) {
	return landingdomain.LandingPage{}, nil
}

func (s *landingDomainPageRepoStub) FindByID(
	context.Context,
	coretenant.Scope,
	string,
) (landingdomain.LandingPage, error) {
	return landingdomain.LandingPage{ID: "page-1"}, nil
}

func (s *landingDomainPageRepoStub) FindBySlug(
	context.Context,
	coretenant.Scope,
	string,
) (landingdomain.LandingPage, error) {
	return landingdomain.LandingPage{ID: "page-1"}, nil
}

func (s *landingDomainPageRepoStub) List(
	context.Context,
	coretenant.Scope,
	repository.PageListFilter,
) ([]landingdomain.LandingPage, int64, error) {
	return nil, 0, nil
}

func (s *landingDomainPageRepoStub) Update(
	context.Context,
	coretenant.Scope,
	string,
	repository.UpdatePageParams,
) (landingdomain.LandingPage, error) {
	return landingdomain.LandingPage{}, nil
}

func (s *landingDomainPageRepoStub) Delete(context.Context, coretenant.Scope, string) error {
	return nil
}

type landingDomainFeatureGateStub struct {
	organizationID string
	featureKey     string
	err            error
}

func (s *landingDomainFeatureGateStub) RequireFeature(
	_ context.Context,
	organizationID string,
	featureKey string,
) (organizationmodel.Entitlement, error) {
	s.organizationID = organizationID
	s.featureKey = featureKey
	return organizationmodel.Entitlement{
		OrganizationID: organizationID,
		FeatureKey:     featureKey,
	}, s.err
}

func TestDomainServiceBindDomainRequiresCustomDomainEntitlement(t *testing.T) {
	repo := &landingDomainRepoStub{
		availableDomains: []landingdomain.AvailableDomain{
			{ID: "domain-1", OrganizationID: landingServiceOrganizationID},
		},
	}
	featureErr := errors.New("feature disabled")
	features := &landingDomainFeatureGateStub{err: featureErr}
	service := NewDomainService(
		repo,
		&landingDomainPageRepoStub{},
		nil,
		WithLandingDomainFeatureGate(features),
	)

	_, err := service.BindDomain(context.Background(), mustLandingScope(t), BindDomainParams{
		OrganizationDomainID: "domain-1",
		LandingPageID:        "page-1",
	})
	if !errors.Is(err, featureErr) {
		t.Fatalf("BindDomain() error = %v, want %v", err, featureErr)
	}
	if repo.bindCalled {
		t.Fatal("BindDomain() should not call repository when feature entitlement fails")
	}
	if features.organizationID != landingServiceOrganizationID ||
		features.featureKey != landingdomain.FeatureLandingCustomDomain {
		t.Fatalf("features = %#v", features)
	}
}

func TestDomainServiceBindDomainAllowsFeatureEnabled(t *testing.T) {
	repo := &landingDomainRepoStub{
		availableDomains: []landingdomain.AvailableDomain{
			{ID: "domain-1", OrganizationID: landingServiceOrganizationID},
		},
	}
	features := &landingDomainFeatureGateStub{}
	service := NewDomainService(
		repo,
		&landingDomainPageRepoStub{},
		nil,
		WithLandingDomainFeatureGate(features),
	)

	result, err := service.BindDomain(context.Background(), mustLandingScope(t), BindDomainParams{
		OrganizationDomainID: "domain-1",
		LandingPageID:        "page-1",
	})
	if err != nil {
		t.Fatalf("BindDomain() error = %v", err)
	}
	if result.ID != "binding-1" || !repo.bindCalled {
		t.Fatalf("result = %#v repo=%#v", result, repo)
	}
}

func TestDomainServiceListAllBindingsPassesThrough(t *testing.T) {
	repo := &landingDomainRepoStub{
		allBindings: []landingdomain.DomainBinding{
			{ID: "binding-1", LandingPageID: "page-1"},
			{ID: "binding-2", LandingPageID: "page-2"},
		},
	}
	service := NewDomainService(repo, &landingDomainPageRepoStub{}, nil)

	bindings, err := service.ListAllBindings(context.Background(), mustLandingScope(t))
	if err != nil {
		t.Fatalf("ListAllBindings() error = %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings across all pages, got %d", len(bindings))
	}
}
