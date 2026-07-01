package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type templateServiceReusableRepoStub struct {
	template domain.SectionTemplate
	err      error
}

func (s *templateServiceReusableRepoStub) CreateSectionTemplate(
	context.Context,
	coretenant.Scope,
	repository.CreateSectionTemplateParams,
) (domain.SectionTemplate, error) {
	return domain.SectionTemplate{}, nil
}

func (s *templateServiceReusableRepoStub) GetSectionTemplate(
	context.Context,
	coretenant.Scope,
	string,
) (domain.SectionTemplate, error) {
	return s.template, s.err
}

func (s *templateServiceReusableRepoStub) ListSectionTemplates(
	context.Context,
	coretenant.Scope,
	domain.SectionType,
) ([]domain.SectionTemplate, error) {
	return nil, nil
}

func (s *templateServiceReusableRepoStub) UpdateSectionTemplate(
	context.Context,
	coretenant.Scope,
	string,
	repository.UpdateSectionTemplateParams,
) (domain.SectionTemplate, error) {
	return domain.SectionTemplate{}, nil
}

func (s *templateServiceReusableRepoStub) DeleteSectionTemplate(
	context.Context,
	coretenant.Scope,
	string,
	string,
) error {
	return nil
}

func (s *templateServiceReusableRepoStub) CreateCTA(
	context.Context,
	coretenant.Scope,
	repository.CreateCTAParams,
) (domain.LandingCTA, error) {
	return domain.LandingCTA{}, nil
}

func (s *templateServiceReusableRepoStub) GetCTA(
	context.Context,
	coretenant.Scope,
	string,
) (domain.LandingCTA, error) {
	return domain.LandingCTA{}, nil
}

func (s *templateServiceReusableRepoStub) ListCTAs(
	context.Context,
	coretenant.Scope,
) ([]domain.LandingCTA, error) {
	return nil, nil
}

func (s *templateServiceReusableRepoStub) UpdateCTA(
	context.Context,
	coretenant.Scope,
	string,
	repository.UpdateCTAParams,
) (domain.LandingCTA, error) {
	return domain.LandingCTA{}, nil
}

func (s *templateServiceReusableRepoStub) DeleteCTA(
	context.Context,
	coretenant.Scope,
	string,
	string,
) error {
	return nil
}

func (s *templateServiceReusableRepoStub) CreateMenu(
	context.Context,
	coretenant.Scope,
	repository.CreateMenuParams,
) (domain.LandingMenu, error) {
	return domain.LandingMenu{}, nil
}

func (s *templateServiceReusableRepoStub) GetMenu(
	context.Context,
	coretenant.Scope,
	string,
) (domain.LandingMenu, error) {
	return domain.LandingMenu{}, nil
}

func (s *templateServiceReusableRepoStub) ListMenus(
	context.Context,
	coretenant.Scope,
) ([]domain.LandingMenu, error) {
	return nil, nil
}

func (s *templateServiceReusableRepoStub) UpdateMenu(
	context.Context,
	coretenant.Scope,
	string,
	repository.UpdateMenuParams,
) (domain.LandingMenu, error) {
	return domain.LandingMenu{}, nil
}

func (s *templateServiceReusableRepoStub) DeleteMenu(
	context.Context,
	coretenant.Scope,
	string,
	string,
) error {
	return nil
}

func (s *templateServiceReusableRepoStub) CreateMenuItem(
	context.Context,
	coretenant.Scope,
	repository.CreateMenuItemParams,
) (domain.LandingMenuItem, error) {
	return domain.LandingMenuItem{}, nil
}

func (s *templateServiceReusableRepoStub) GetMenuItem(
	context.Context,
	coretenant.Scope,
	string,
) (domain.LandingMenuItem, error) {
	return domain.LandingMenuItem{}, nil
}

func (s *templateServiceReusableRepoStub) ListMenuItems(
	context.Context,
	coretenant.Scope,
	string,
) ([]domain.LandingMenuItem, error) {
	return nil, nil
}

func (s *templateServiceReusableRepoStub) UpdateMenuItem(
	context.Context,
	coretenant.Scope,
	string,
	repository.UpdateMenuItemParams,
) (domain.LandingMenuItem, error) {
	return domain.LandingMenuItem{}, nil
}

func (s *templateServiceReusableRepoStub) ReorderMenuItems(
	context.Context,
	coretenant.Scope,
	string,
	[]string,
) error {
	return nil
}

func (s *templateServiceReusableRepoStub) DeleteMenuItem(
	context.Context,
	coretenant.Scope,
	string,
) error {
	return nil
}

func TestTemplateServiceInstantiateToPageUsesSectionQuotaGuard(t *testing.T) {
	reusableRepo := &templateServiceReusableRepoStub{
		template: domain.SectionTemplate{
			ID:          "template-1",
			Name:        "Hero Template",
			SectionType: domain.SectionTypeHero,
			Content:     map[string]any{"title": "Hello"},
			Style:       map[string]any{"color": "red"},
		},
	}
	sectionRepo := &sectionServiceRepoStub{
		listByPageItems: []domain.LandingSection{
			{ID: "section-1"},
			{ID: "section-2"},
		},
	}
	guard := &sectionServiceQuotaGuardStub{}
	service := NewTemplateService(
		reusableRepo,
		sectionRepo,
		WithTemplateSectionQuotaGuard(guard),
	)

	_, err := service.InstantiateToPage(
		context.Background(),
		mustLandingScope(t),
		"template-1",
		"page-1",
		"hero-3",
		2,
		"user-1",
	)
	if err != nil {
		t.Fatalf("InstantiateToPage() error = %v", err)
	}
	if !sectionRepo.createCalled {
		t.Fatal("InstantiateToPage() should persist section when quota allows")
	}
	if guard.organizationID != landingServiceOrganizationID ||
		guard.featureKey != domain.FeatureLandingMaxSections ||
		guard.limitKey != "limit" ||
		guard.usedValue != 2 ||
		guard.delta != 1 {
		t.Fatalf("guard = %#v", guard)
	}
}

func TestTemplateServiceInstantiateToPageStopsWhenQuotaExceeded(t *testing.T) {
	reusableRepo := &templateServiceReusableRepoStub{
		template: domain.SectionTemplate{
			ID:          "template-1",
			Name:        "Hero Template",
			SectionType: domain.SectionTypeHero,
		},
	}
	sectionRepo := &sectionServiceRepoStub{}
	guardErr := errors.New("quota exceeded")
	service := NewTemplateService(
		reusableRepo,
		sectionRepo,
		WithTemplateSectionQuotaGuard(&sectionServiceQuotaGuardStub{err: guardErr}),
	)

	_, err := service.InstantiateToPage(
		context.Background(),
		mustLandingScope(t),
		"template-1",
		"page-1",
		"hero-1",
		0,
		"user-1",
	)
	if !errors.Is(err, guardErr) {
		t.Fatalf("InstantiateToPage() error = %v, want %v", err, guardErr)
	}
	if sectionRepo.createCalled {
		t.Fatal("InstantiateToPage() should not persist section when quota guard fails")
	}
}
