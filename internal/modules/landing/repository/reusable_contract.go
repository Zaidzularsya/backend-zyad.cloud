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

type UpdateSectionTemplateParams struct {
	Name        *string
	Description *string
	Content     map[string]any
	Style       map[string]any
	UpdatedBy   string
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

type UpdateCTAParams struct {
	Name        *string
	Label       *string
	Type        *domain.CTAType
	Target      *domain.CTATarget
	Destination *string
	TrackingKey *string
	UpdatedBy   string
}

type CreatePricingPlanParams struct {
	Name          string
	PriceLabel    string
	IntervalLabel string
	Description   string
	Features      []string
	CTALabel      string
	CTAURL        string
	IsFeatured    bool
	IsEnabled     bool
	CreatedBy     string
}

type UpdatePricingPlanParams struct {
	Name          *string
	PriceLabel    *string
	IntervalLabel *string
	Description   *string
	Features      *[]string
	CTALabel      *string
	CTAURL        *string
	IsFeatured    *bool
	IsEnabled     *bool
	UpdatedBy     string
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

type UpdateMenuParams struct {
	Name      *string
	Location  *domain.MenuLocation
	IsActive  *bool
	UpdatedBy string
}

type UpdateMenuItemParams struct {
	ParentID    *string
	Label       *string
	LinkType    *domain.LinkType
	Destination *string
	Target      *domain.CTATarget
	SortOrder   *int
	IsEnabled   *bool
}

type ReusableRepository interface {
	// Section Templates
	CreateSectionTemplate(context.Context, coretenant.Scope, CreateSectionTemplateParams) (domain.SectionTemplate, error)
	GetSectionTemplate(context.Context, coretenant.Scope, string) (domain.SectionTemplate, error)
	ListSectionTemplates(context.Context, coretenant.Scope, domain.SectionType) ([]domain.SectionTemplate, error)
	UpdateSectionTemplate(context.Context, coretenant.Scope, string, UpdateSectionTemplateParams) (domain.SectionTemplate, error)
	DeleteSectionTemplate(context.Context, coretenant.Scope, string, string) error

	// CTAs
	CreateCTA(context.Context, coretenant.Scope, CreateCTAParams) (domain.LandingCTA, error)
	GetCTA(context.Context, coretenant.Scope, string) (domain.LandingCTA, error)
	ListCTAs(context.Context, coretenant.Scope) ([]domain.LandingCTA, error)
	UpdateCTA(context.Context, coretenant.Scope, string, UpdateCTAParams) (domain.LandingCTA, error)
	DeleteCTA(context.Context, coretenant.Scope, string, string) error

	// Pricing Plans
	CreatePricingPlan(context.Context, coretenant.Scope, CreatePricingPlanParams) (domain.LandingPricingPlan, error)
	GetPricingPlan(context.Context, coretenant.Scope, string) (domain.LandingPricingPlan, error)
	ListPricingPlans(context.Context, coretenant.Scope) ([]domain.LandingPricingPlan, error)
	UpdatePricingPlan(context.Context, coretenant.Scope, string, UpdatePricingPlanParams) (domain.LandingPricingPlan, error)
	ReorderPricingPlans(context.Context, coretenant.Scope, []string) error
	DeletePricingPlan(context.Context, coretenant.Scope, string, string) error

	// Menus
	CreateMenu(context.Context, coretenant.Scope, CreateMenuParams) (domain.LandingMenu, error)
	GetMenu(context.Context, coretenant.Scope, string) (domain.LandingMenu, error)
	ListMenus(context.Context, coretenant.Scope) ([]domain.LandingMenu, error)
	UpdateMenu(context.Context, coretenant.Scope, string, UpdateMenuParams) (domain.LandingMenu, error)
	DeleteMenu(context.Context, coretenant.Scope, string, string) error

	// Menu Items
	CreateMenuItem(context.Context, coretenant.Scope, CreateMenuItemParams) (domain.LandingMenuItem, error)
	GetMenuItem(context.Context, coretenant.Scope, string) (domain.LandingMenuItem, error)
	ListMenuItems(context.Context, coretenant.Scope, string) ([]domain.LandingMenuItem, error)
	UpdateMenuItem(context.Context, coretenant.Scope, string, UpdateMenuItemParams) (domain.LandingMenuItem, error)
	ReorderMenuItems(context.Context, coretenant.Scope, string, []string) error
	DeleteMenuItem(context.Context, coretenant.Scope, string) error
}
