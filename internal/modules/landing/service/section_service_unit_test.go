package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	landingdomain "zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type sectionServiceRepoStub struct {
	listByPageItems []landingdomain.LandingSection
	listByPageErr   error
	createCalled    bool
	updateCalled    bool
	findByIDResult  landingdomain.LandingSection
	findByIDErr     error
}

func (s *sectionServiceRepoStub) Create(
	context.Context,
	coretenant.Scope,
	repository.CreateSectionParams,
) (landingdomain.LandingSection, error) {
	s.createCalled = true
	return landingdomain.LandingSection{
		ID:            "section-1",
		LandingPageID: "page-1",
	}, nil
}

func (s *sectionServiceRepoStub) FindByID(
	context.Context,
	coretenant.Scope,
	string,
) (landingdomain.LandingSection, error) {
	return s.findByIDResult, s.findByIDErr
}

func (s *sectionServiceRepoStub) ListByPage(
	context.Context,
	coretenant.Scope,
	string,
) ([]landingdomain.LandingSection, error) {
	return s.listByPageItems, s.listByPageErr
}

func (s *sectionServiceRepoStub) Update(
	context.Context,
	coretenant.Scope,
	string,
	repository.UpdateSectionParams,
) (landingdomain.LandingSection, error) {
	s.updateCalled = true
	return landingdomain.LandingSection{}, nil
}

func (s *sectionServiceRepoStub) Reorder(
	context.Context,
	coretenant.Scope,
	string,
	[]repository.SectionReorderParam,
) error {
	return nil
}

func (s *sectionServiceRepoStub) Delete(context.Context, coretenant.Scope, string) error {
	return nil
}

type sectionServiceQuotaGuardStub struct {
	organizationID string
	featureKey     string
	limitKey       string
	usedValue      int64
	delta          int64
	err            error
}

func (s *sectionServiceQuotaGuardStub) RequireQuotaValue(
	_ context.Context,
	organizationID string,
	featureKey string,
	limitKey string,
	usedValue int64,
	delta int64,
) error {
	s.organizationID = organizationID
	s.featureKey = featureKey
	s.limitKey = limitKey
	s.usedValue = usedValue
	s.delta = delta
	return s.err
}

func TestSectionServiceCreateUsesBillingQuotaGuard(t *testing.T) {
	repo := &sectionServiceRepoStub{
		listByPageItems: []landingdomain.LandingSection{
			{ID: "section-1"},
			{ID: "section-2"},
		},
	}
	guard := &sectionServiceQuotaGuardStub{}
	service := NewSectionService(repo, WithLandingSectionQuotaGuard(guard))

	_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, repository.CreateSectionParams{
		LandingPageID: "page-1",
		Key:           "hero-3",
		Type:          landingdomain.SectionTypeHero,
		Name:          "Hero",
		IsEnabled:     true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !repo.createCalled {
		t.Fatal("Create() should persist section when quota allows")
	}
	if guard.organizationID != landingServiceOrganizationID ||
		guard.featureKey != landingdomain.FeatureLandingMaxSections ||
		guard.limitKey != "limit" ||
		guard.usedValue != 2 ||
		guard.delta != 1 {
		t.Fatalf("guard = %#v", guard)
	}
}

func TestSectionServiceCreateStopsWhenQuotaExceeded(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	guardErr := errors.New("quota exceeded")
	service := NewSectionService(
		repo,
		WithLandingSectionQuotaGuard(&sectionServiceQuotaGuardStub{err: guardErr}),
	)

	_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, repository.CreateSectionParams{
		LandingPageID: "page-1",
		Key:           "hero-1",
		Type:          landingdomain.SectionTypeHero,
		Name:          "Hero",
		IsEnabled:     true,
	})
	if !errors.Is(err, guardErr) {
		t.Fatalf("Create() error = %v, want %v", err, guardErr)
	}
	if repo.createCalled {
		t.Fatal("Create() should not persist section when quota guard fails")
	}
}

func TestSectionServiceCreateRejectsPlatformCatalogForNonPlatformOrg(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	service := NewSectionService(repo)

	_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, repository.CreateSectionParams{
		LandingPageID: "page-1",
		Key:           "pricing-1",
		Type:          landingdomain.SectionTypePricing,
		Name:          "Pricing",
		IsEnabled:     true,
		Content:       map[string]any{"source": "platform_catalog"},
	})
	if err == nil {
		t.Fatal("Create() expected error for non-platform organization using platform_catalog source")
	}
	if repo.createCalled {
		t.Fatal("Create() should not persist section when pricing source is not allowed")
	}
}

func TestSectionServiceCreateAllowsPlatformCatalogForPlatformOrg(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	service := NewSectionService(repo)

	_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypePlatform, repository.CreateSectionParams{
		LandingPageID: "page-1",
		Key:           "pricing-1",
		Type:          landingdomain.SectionTypePricing,
		Name:          "Pricing",
		IsEnabled:     true,
		Content:       map[string]any{"source": "platform_catalog"},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error = %v", err)
	}
	if !repo.createCalled {
		t.Fatal("Create() should persist section when platform organization uses platform_catalog source")
	}
}

func TestSectionServiceCreateAllowsCustomPricingForNonPlatformOrg(t *testing.T) {
	repo := &sectionServiceRepoStub{}
	service := NewSectionService(repo)

	_, err := service.Create(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, repository.CreateSectionParams{
		LandingPageID: "page-1",
		Key:           "pricing-1",
		Type:          landingdomain.SectionTypePricing,
		Name:          "Pricing",
		IsEnabled:     true,
		Content:       map[string]any{"source": "custom"},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error = %v", err)
	}
	if !repo.createCalled {
		t.Fatal("Create() should persist section when using custom pricing source")
	}
}

func TestSectionServiceUpdateRejectsPlatformCatalogForNonPlatformOrg(t *testing.T) {
	repo := &sectionServiceRepoStub{
		findByIDResult: landingdomain.LandingSection{
			ID:   "section-1",
			Type: landingdomain.SectionTypePricing,
		},
	}
	service := NewSectionService(repo)

	_, err := service.Update(context.Background(), mustLandingScope(t), coretenant.OrganizationTypeCustomer, "section-1", repository.UpdateSectionParams{
		Content: map[string]any{"source": "platform_catalog"},
	})
	if err == nil {
		t.Fatal("Update() expected error for non-platform organization using platform_catalog source")
	}
	if repo.updateCalled {
		t.Fatal("Update() should not persist section when pricing source is not allowed")
	}
}

func TestSectionServiceUpdateAllowsPlatformCatalogForPlatformOrg(t *testing.T) {
	repo := &sectionServiceRepoStub{
		findByIDResult: landingdomain.LandingSection{
			ID:   "section-1",
			Type: landingdomain.SectionTypePricing,
		},
	}
	service := NewSectionService(repo)

	_, err := service.Update(context.Background(), mustLandingScope(t), coretenant.OrganizationTypePlatform, "section-1", repository.UpdateSectionParams{
		Content: map[string]any{"source": "platform_catalog"},
	})
	if err != nil {
		t.Fatalf("Update() unexpected error = %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("Update() should persist section when platform organization uses platform_catalog source")
	}
}
