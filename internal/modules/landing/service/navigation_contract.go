package service

import (
	"context"
	"errors"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

var (
	ErrInvalidMenuLocation = errors.New("invalid menu location")
	ErrInvalidLinkType     = errors.New("invalid link type")
	ErrMaxMenuDepth        = errors.New("maximum menu depth exceeded")
)

const MaxMenuDepth = 3

// NavigationService interface defines operations for Landing Menus and Menu Items
type NavigationService interface {
	// Menus
	CreateMenu(ctx context.Context, scope tenant.Scope, params repository.CreateMenuParams) (domain.LandingMenu, error)
	FindMenuByID(ctx context.Context, scope tenant.Scope, id string) (domain.LandingMenu, error)
	ListMenus(ctx context.Context, scope tenant.Scope) ([]domain.LandingMenu, error)
	UpdateMenu(ctx context.Context, scope tenant.Scope, id string, params repository.UpdateMenuParams) (domain.LandingMenu, error)
	DeleteMenu(ctx context.Context, scope tenant.Scope, id string, updatedBy string) error

	// Menu Items
	CreateMenuItem(ctx context.Context, scope tenant.Scope, params repository.CreateMenuItemParams) (domain.LandingMenuItem, error)
	FindMenuItemByID(ctx context.Context, scope tenant.Scope, id string) (domain.LandingMenuItem, error)
	ListMenuItems(ctx context.Context, scope tenant.Scope, menuID string) ([]domain.LandingMenuItem, error)
	UpdateMenuItem(ctx context.Context, scope tenant.Scope, id string, params repository.UpdateMenuItemParams) (domain.LandingMenuItem, error)
	ReorderMenuItems(ctx context.Context, scope tenant.Scope, menuID string, itemIDs []string) error
	DeleteMenuItem(ctx context.Context, scope tenant.Scope, id string) error
}
