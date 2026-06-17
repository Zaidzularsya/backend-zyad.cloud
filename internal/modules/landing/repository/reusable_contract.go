package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type CreateSectionTemplateParams struct {
	Name        string
	Description string
	SectionType domain.SectionType
	Content     map[string]any
	Style       map[string]any
	CreatedBy   string
}

type CreateCTAParams struct {
	Name        string
	Label       string
	Type        domain.CTAType
	Target      domain.CTATarget
	Destination string
	TrackingKey string
	CreatedBy   string
}

type CreateMenuParams struct {
	Name      string
	Location  domain.MenuLocation
	IsActive  bool
	CreatedBy string
}

type CreateMenuItemParams struct {
	MenuID      string
	ParentID    *string
	Label       string
	LinkType    domain.LinkType
	Destination string
	Target      domain.CTATarget
	SortOrder   int
	IsEnabled   bool
}

type ReusableRepository interface {
	// Section Templates
	CreateSectionTemplate(context.Context, coretenant.Scope, CreateSectionTemplateParams) (domain.SectionTemplate, error)
	GetSectionTemplate(context.Context, coretenant.Scope, string) (domain.SectionTemplate, error)
	ListSectionTemplates(context.Context, coretenant.Scope, domain.SectionType) ([]domain.SectionTemplate, error)
	DeleteSectionTemplate(context.Context, coretenant.Scope, string, string) error

	// CTAs
	CreateCTA(context.Context, coretenant.Scope, CreateCTAParams) (domain.LandingCTA, error)
	GetCTA(context.Context, coretenant.Scope, string) (domain.LandingCTA, error)
	ListCTAs(context.Context, coretenant.Scope) ([]domain.LandingCTA, error)
	DeleteCTA(context.Context, coretenant.Scope, string, string) error

	// Menus
	CreateMenu(context.Context, coretenant.Scope, CreateMenuParams) (domain.LandingMenu, error)
	GetMenu(context.Context, coretenant.Scope, string) (domain.LandingMenu, error)
	ListMenus(context.Context, coretenant.Scope) ([]domain.LandingMenu, error)
	DeleteMenu(context.Context, coretenant.Scope, string, string) error

	// Menu Items
	CreateMenuItem(context.Context, coretenant.Scope, CreateMenuItemParams) (domain.LandingMenuItem, error)
	ListMenuItems(context.Context, coretenant.Scope, string) ([]domain.LandingMenuItem, error)
	ReorderMenuItems(context.Context, coretenant.Scope, string, []string) error
	DeleteMenuItem(context.Context, coretenant.Scope, string) error
}
