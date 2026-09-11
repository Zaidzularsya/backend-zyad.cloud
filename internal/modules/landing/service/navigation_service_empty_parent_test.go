package service

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type fakeNavigationRepository struct {
	updateCalled      bool
	getMenuItemResult domain.LandingMenuItem
}

func (f *fakeNavigationRepository) CreateSectionTemplate(context.Context, coretenant.Scope, repository.CreateSectionTemplateParams) (domain.SectionTemplate, error) {
	return domain.SectionTemplate{}, nil
}

func (f *fakeNavigationRepository) GetSectionTemplate(context.Context, coretenant.Scope, string) (domain.SectionTemplate, error) {
	return domain.SectionTemplate{}, nil
}

func (f *fakeNavigationRepository) ListSectionTemplates(context.Context, coretenant.Scope, domain.SectionType) ([]domain.SectionTemplate, error) {
	return nil, nil
}

func (f *fakeNavigationRepository) UpdateSectionTemplate(context.Context, coretenant.Scope, string, repository.UpdateSectionTemplateParams) (domain.SectionTemplate, error) {
	return domain.SectionTemplate{}, nil
}

func (f *fakeNavigationRepository) DeleteSectionTemplate(context.Context, coretenant.Scope, string, string) error {
	return nil
}

func (f *fakeNavigationRepository) CreateCTA(context.Context, coretenant.Scope, repository.CreateCTAParams) (domain.LandingCTA, error) {
	return domain.LandingCTA{}, nil
}

func (f *fakeNavigationRepository) GetCTA(context.Context, coretenant.Scope, string) (domain.LandingCTA, error) {
	return domain.LandingCTA{}, nil
}

func (f *fakeNavigationRepository) ListCTAs(context.Context, coretenant.Scope) ([]domain.LandingCTA, error) {
	return nil, nil
}

func (f *fakeNavigationRepository) UpdateCTA(context.Context, coretenant.Scope, string, repository.UpdateCTAParams) (domain.LandingCTA, error) {
	return domain.LandingCTA{}, nil
}

func (f *fakeNavigationRepository) DeleteCTA(context.Context, coretenant.Scope, string, string) error {
	return nil
}

func (f *fakeNavigationRepository) CreatePricingPlan(context.Context, coretenant.Scope, repository.CreatePricingPlanParams) (domain.LandingPricingPlan, error) {
	return domain.LandingPricingPlan{}, nil
}

func (f *fakeNavigationRepository) GetPricingPlan(context.Context, coretenant.Scope, string) (domain.LandingPricingPlan, error) {
	return domain.LandingPricingPlan{}, nil
}

func (f *fakeNavigationRepository) ListPricingPlans(context.Context, coretenant.Scope) ([]domain.LandingPricingPlan, error) {
	return nil, nil
}

func (f *fakeNavigationRepository) UpdatePricingPlan(context.Context, coretenant.Scope, string, repository.UpdatePricingPlanParams) (domain.LandingPricingPlan, error) {
	return domain.LandingPricingPlan{}, nil
}

func (f *fakeNavigationRepository) ReorderPricingPlans(context.Context, coretenant.Scope, []string) error {
	return nil
}

func (f *fakeNavigationRepository) DeletePricingPlan(context.Context, coretenant.Scope, string, string) error {
	return nil
}

func (f *fakeNavigationRepository) CreateMenu(context.Context, coretenant.Scope, repository.CreateMenuParams) (domain.LandingMenu, error) {
	return domain.LandingMenu{}, nil
}

func (f *fakeNavigationRepository) GetMenu(context.Context, coretenant.Scope, string) (domain.LandingMenu, error) {
	return domain.LandingMenu{}, nil
}

func (f *fakeNavigationRepository) ListMenus(context.Context, coretenant.Scope) ([]domain.LandingMenu, error) {
	return nil, nil
}

func (f *fakeNavigationRepository) UpdateMenu(context.Context, coretenant.Scope, string, repository.UpdateMenuParams) (domain.LandingMenu, error) {
	return domain.LandingMenu{}, nil
}

func (f *fakeNavigationRepository) DeleteMenu(context.Context, coretenant.Scope, string, string) error {
	return nil
}

func (f *fakeNavigationRepository) CreateMenuItem(context.Context, coretenant.Scope, repository.CreateMenuItemParams) (domain.LandingMenuItem, error) {
	return domain.LandingMenuItem{}, nil
}

func (f *fakeNavigationRepository) GetMenuItem(context.Context, coretenant.Scope, string) (domain.LandingMenuItem, error) {
	return f.getMenuItemResult, nil
}

func (f *fakeNavigationRepository) ListMenuItems(context.Context, coretenant.Scope, string) ([]domain.LandingMenuItem, error) {
	return nil, nil
}

func (f *fakeNavigationRepository) UpdateMenuItem(context.Context, coretenant.Scope, string, repository.UpdateMenuItemParams) (domain.LandingMenuItem, error) {
	f.updateCalled = true
	return domain.LandingMenuItem{}, nil
}

func (f *fakeNavigationRepository) ReorderMenuItems(context.Context, coretenant.Scope, string, []string) error {
	return nil
}

func (f *fakeNavigationRepository) DeleteMenuItem(context.Context, coretenant.Scope, string) error {
	return nil
}

func TestUpdateMenuItemIgnoresEmptyParentID(t *testing.T) {
	repo := &fakeNavigationRepository{}
	svc := NewNavigationService(repo, nil)

	scope, err := coretenant.NewScope(mustTenantContext())
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}

	_, err = svc.UpdateMenuItem(context.Background(), scope, "item-1", repository.UpdateMenuItemParams{
		ParentID: &[]string{""}[0],
	})
	if err != nil {
		t.Fatalf("UpdateMenuItem() error = %v", err)
	}
	if !repo.updateCalled {
		t.Fatalf("UpdateMenuItem() did not reach repository")
	}
}

func mustTenantContext() coretenant.Context {
	ctx, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     "11111111-1111-1111-1111-111111111111",
		OrganizationSlug:   "organization-a",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceInternal,
		DataPlacement:      coretenant.DataPlacementShared,
		RequestHost:        "example.test",
	})
	if err != nil {
		panic(err)
	}
	return ctx
}
