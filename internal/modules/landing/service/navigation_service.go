package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type navigationService struct {
	repo     repository.ReusableRepository
	pageRepo repository.PageRepository
}

func NewNavigationService(repo repository.ReusableRepository, pageRepo repository.PageRepository) NavigationService {
	return &navigationService{
		repo:     repo,
		pageRepo: pageRepo,
	}
}

// validateInternalPageDestination confirms destination resolves, by slug, to
// a real page in the same tenant. Internal-page menu items store the target
// page's slug (not its ID) as Destination. This is a write-time existence
// check only — whether the target is currently published is a render-time
// concern for the public resolver, not an authoring-time restriction.
func (s *navigationService) validateInternalPageDestination(ctx context.Context, scope tenant.Scope, destination string) error {
	if _, err := s.pageRepo.FindBySlug(ctx, scope, destination); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidInternalPageDestination
		}
		return err
	}
	return nil
}

// Menus
func (s *navigationService) CreateMenu(ctx context.Context, scope tenant.Scope, params repository.CreateMenuParams) (domain.LandingMenu, error) {
	if !scope.IsValid() {
		return domain.LandingMenu{}, tenant.ErrInvalidScope
	}

	if !params.Location.IsValid() {
		return domain.LandingMenu{}, ErrInvalidMenuLocation
	}

	return s.repo.CreateMenu(ctx, scope, params)
}

func (s *navigationService) FindMenuByID(ctx context.Context, scope tenant.Scope, id string) (domain.LandingMenu, error) {
	if !scope.IsValid() {
		return domain.LandingMenu{}, tenant.ErrInvalidScope
	}
	return s.repo.GetMenu(ctx, scope, id)
}

func (s *navigationService) ListMenus(ctx context.Context, scope tenant.Scope) ([]domain.LandingMenu, error) {
	if !scope.IsValid() {
		return nil, tenant.ErrInvalidScope
	}
	return s.repo.ListMenus(ctx, scope)
}

func (s *navigationService) UpdateMenu(ctx context.Context, scope tenant.Scope, id string, params repository.UpdateMenuParams) (domain.LandingMenu, error) {
	if !scope.IsValid() {
		return domain.LandingMenu{}, tenant.ErrInvalidScope
	}

	if params.Location != nil && !params.Location.IsValid() {
		return domain.LandingMenu{}, ErrInvalidMenuLocation
	}

	return s.repo.UpdateMenu(ctx, scope, id, params)
}

func (s *navigationService) DeleteMenu(ctx context.Context, scope tenant.Scope, id string, updatedBy string) error {
	if !scope.IsValid() {
		return tenant.ErrInvalidScope
	}
	return s.repo.DeleteMenu(ctx, scope, id, updatedBy)
}

// Menu Items
func (s *navigationService) CreateMenuItem(ctx context.Context, scope tenant.Scope, params repository.CreateMenuItemParams) (domain.LandingMenuItem, error) {
	if !scope.IsValid() {
		return domain.LandingMenuItem{}, tenant.ErrInvalidScope
	}

	if !params.LinkType.IsValid() {
		return domain.LandingMenuItem{}, ErrInvalidLinkType
	}

	if params.LinkType == domain.LinkTypeInternalPage {
		if err := s.validateInternalPageDestination(ctx, scope, params.Destination); err != nil {
			return domain.LandingMenuItem{}, err
		}
	}

	if params.ParentID != nil && *params.ParentID != "" {
		if err := s.checkDepth(ctx, scope, *params.ParentID, 1); err != nil {
			return domain.LandingMenuItem{}, err
		}
	}

	return s.repo.CreateMenuItem(ctx, scope, params)
}

func (s *navigationService) FindMenuItemByID(ctx context.Context, scope tenant.Scope, id string) (domain.LandingMenuItem, error) {
	if !scope.IsValid() {
		return domain.LandingMenuItem{}, tenant.ErrInvalidScope
	}
	return s.repo.GetMenuItem(ctx, scope, id)
}

func (s *navigationService) ListMenuItems(ctx context.Context, scope tenant.Scope, menuID string) ([]domain.LandingMenuItem, error) {
	if !scope.IsValid() {
		return nil, tenant.ErrInvalidScope
	}
	return s.repo.ListMenuItems(ctx, scope, menuID)
}

func (s *navigationService) UpdateMenuItem(ctx context.Context, scope tenant.Scope, id string, params repository.UpdateMenuItemParams) (domain.LandingMenuItem, error) {
	if !scope.IsValid() {
		return domain.LandingMenuItem{}, tenant.ErrInvalidScope
	}

	if params.LinkType != nil && !params.LinkType.IsValid() {
		return domain.LandingMenuItem{}, ErrInvalidLinkType
	}

	if params.ParentID != nil && *params.ParentID != "" {
		// Cannot be its own parent
		if *params.ParentID == id {
			return domain.LandingMenuItem{}, ErrMaxMenuDepth
		}

		if err := s.checkDepth(ctx, scope, *params.ParentID, 1); err != nil {
			return domain.LandingMenuItem{}, err
		}
	}

	if params.Destination != nil {
		linkType := params.LinkType
		if linkType == nil {
			existing, err := s.repo.GetMenuItem(ctx, scope, id)
			if err != nil {
				return domain.LandingMenuItem{}, err
			}
			linkType = &existing.LinkType
		}
		if *linkType == domain.LinkTypeInternalPage {
			if err := s.validateInternalPageDestination(ctx, scope, *params.Destination); err != nil {
				return domain.LandingMenuItem{}, err
			}
		}
	}

	return s.repo.UpdateMenuItem(ctx, scope, id, params)
}

func (s *navigationService) ReorderMenuItems(ctx context.Context, scope tenant.Scope, menuID string, itemIDs []string) error {
	if !scope.IsValid() {
		return tenant.ErrInvalidScope
	}
	return s.repo.ReorderMenuItems(ctx, scope, menuID, itemIDs)
}

func (s *navigationService) DeleteMenuItem(ctx context.Context, scope tenant.Scope, id string) error {
	if !scope.IsValid() {
		return tenant.ErrInvalidScope
	}
	return s.repo.DeleteMenuItem(ctx, scope, id)
}

func (s *navigationService) checkDepth(ctx context.Context, scope tenant.Scope, itemID string, currentDepth int) error {
	if currentDepth >= MaxMenuDepth {
		return ErrMaxMenuDepth
	}

	item, err := s.repo.GetMenuItem(ctx, scope, itemID)
	if err != nil {
		return err // either not found or db error
	}

	if item.ParentID != nil {
		return s.checkDepth(ctx, scope, *item.ParentID, currentDepth+1)
	}

	return nil
}
