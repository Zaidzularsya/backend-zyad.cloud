package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type fakeNavigationPageRepo struct {
	slugs map[string]bool
}

func (f *fakeNavigationPageRepo) Create(context.Context, coretenant.Scope, repository.CreatePageParams) (domain.LandingPage, error) {
	return domain.LandingPage{}, nil
}

func (f *fakeNavigationPageRepo) FindByID(context.Context, coretenant.Scope, string) (domain.LandingPage, error) {
	return domain.LandingPage{}, nil
}

func (f *fakeNavigationPageRepo) FindBySlug(_ context.Context, _ coretenant.Scope, slug string) (domain.LandingPage, error) {
	if f.slugs[slug] {
		return domain.LandingPage{Slug: slug}, nil
	}
	return domain.LandingPage{}, pgx.ErrNoRows
}

func (f *fakeNavigationPageRepo) List(context.Context, coretenant.Scope, repository.PageListFilter) ([]domain.LandingPage, int64, error) {
	return nil, 0, nil
}

func (f *fakeNavigationPageRepo) Update(context.Context, coretenant.Scope, string, repository.UpdatePageParams) (domain.LandingPage, error) {
	return domain.LandingPage{}, nil
}

func (f *fakeNavigationPageRepo) Delete(context.Context, coretenant.Scope, string) error {
	return nil
}

func TestCreateMenuItemRejectsUnknownInternalPageSlug(t *testing.T) {
	svc := NewNavigationService(&fakeNavigationRepository{}, &fakeNavigationPageRepo{slugs: map[string]bool{}})

	scope, err := coretenant.NewScope(mustTenantContext())
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}

	_, err = svc.CreateMenuItem(context.Background(), scope, repository.CreateMenuItemParams{
		Label:       "Blog",
		LinkType:    domain.LinkTypeInternalPage,
		Destination: "nonexistent-slug",
	})
	if !errors.Is(err, ErrInvalidInternalPageDestination) {
		t.Fatalf("CreateMenuItem() error = %v, want ErrInvalidInternalPageDestination", err)
	}
}

func TestCreateMenuItemAcceptsKnownInternalPageSlug(t *testing.T) {
	svc := NewNavigationService(&fakeNavigationRepository{}, &fakeNavigationPageRepo{slugs: map[string]bool{"blog": true}})

	scope, err := coretenant.NewScope(mustTenantContext())
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}

	_, err = svc.CreateMenuItem(context.Background(), scope, repository.CreateMenuItemParams{
		Label:       "Blog",
		LinkType:    domain.LinkTypeInternalPage,
		Destination: "blog",
	})
	if err != nil {
		t.Fatalf("CreateMenuItem() error = %v, want nil", err)
	}
}

func TestCreateMenuItemSkipsInternalPageValidationForOtherLinkTypes(t *testing.T) {
	svc := NewNavigationService(&fakeNavigationRepository{}, &fakeNavigationPageRepo{slugs: map[string]bool{}})

	scope, err := coretenant.NewScope(mustTenantContext())
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}

	_, err = svc.CreateMenuItem(context.Background(), scope, repository.CreateMenuItemParams{
		Label:       "Docs",
		LinkType:    domain.LinkTypeExternalLink,
		Destination: "https://example.com/docs",
	})
	if err != nil {
		t.Fatalf("CreateMenuItem() error = %v, want nil for external_link", err)
	}
}

func TestUpdateMenuItemRejectsUnknownInternalPageSlugWhenDestinationChanges(t *testing.T) {
	repo := &fakeNavigationRepository{
		getMenuItemResult: domain.LandingMenuItem{LinkType: domain.LinkTypeInternalPage},
	}
	svc := NewNavigationService(repo, &fakeNavigationPageRepo{slugs: map[string]bool{}})

	scope, err := coretenant.NewScope(mustTenantContext())
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}

	newDestination := "nonexistent-slug"
	_, err = svc.UpdateMenuItem(context.Background(), scope, "item-1", repository.UpdateMenuItemParams{
		Destination: &newDestination,
	})
	if !errors.Is(err, ErrInvalidInternalPageDestination) {
		t.Fatalf("UpdateMenuItem() error = %v, want ErrInvalidInternalPageDestination", err)
	}
	if repo.updateCalled {
		t.Fatal("UpdateMenuItem() should not reach repository when destination is invalid")
	}
}

func TestUpdateMenuItemAllowsValidDestinationChange(t *testing.T) {
	repo := &fakeNavigationRepository{
		getMenuItemResult: domain.LandingMenuItem{LinkType: domain.LinkTypeInternalPage},
	}
	svc := NewNavigationService(repo, &fakeNavigationPageRepo{slugs: map[string]bool{"blog": true}})

	scope, err := coretenant.NewScope(mustTenantContext())
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}

	newDestination := "blog"
	_, err = svc.UpdateMenuItem(context.Background(), scope, "item-1", repository.UpdateMenuItemParams{
		Destination: &newDestination,
	})
	if err != nil {
		t.Fatalf("UpdateMenuItem() error = %v, want nil", err)
	}
	if !repo.updateCalled {
		t.Fatal("UpdateMenuItem() should reach repository when destination is valid")
	}
}
