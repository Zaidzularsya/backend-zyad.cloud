package service

import (
	"context"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type ResolvedPage struct {
	Page         domain.LandingPage
	Branding     domain.LandingBranding
	Sections     []domain.LandingSection
	Forms        []domain.LandingForm
	Menus        []ResolvedMenu
	CTAs         []ResolvedCTA
	PricingPlans []ResolvedPricingPlan
	Snapshot     map[string]any
	IsDraft      bool

	// Set only for pages authored with the GrapesJS builder. When Builder is
	// "grapesjs" the renderer ignores Sections/Forms and uses HTML+CSS instead.
	Builder string
	HTML    string
	CSS     string
}

// ResolvedCTA is the public-safe projection of domain.LandingCTA, exposed so the
// renderer can resolve PageSettings.SecondaryCTATrackingKey to a label/url
// without a separate authenticated request (FTR-FE-014).
type ResolvedCTA struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Target      string `json:"target"`
	Destination string `json:"destination"`
	TrackingKey string `json:"tracking_key"`
}

// ResolvedPricingPlan is the public-safe projection of a tenant's own
// domain.LandingPricingPlan, used to fill the `zyad-pricing-plans` block
// (data-zyad-slot="pricing-plans") client-side.
type ResolvedPricingPlan struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	PriceLabel    string   `json:"price_label"`
	IntervalLabel string   `json:"interval_label,omitempty"`
	Description   string   `json:"description,omitempty"`
	Features      []string `json:"features"`
	CTALabel      string   `json:"cta_label"`
	CTAURL        string   `json:"cta_url,omitempty"`
	IsFeatured    bool     `json:"is_featured"`
}

type ResolvedMenu struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Location string             `json:"location"`
	IsActive bool               `json:"is_active"`
	Items    []ResolvedMenuItem `json:"items"`
}

type ResolvedMenuItem struct {
	ID          string             `json:"id"`
	ParentID    *string            `json:"parent_id,omitempty"`
	Label       string             `json:"label"`
	LinkType    string             `json:"link_type"`
	Destination string             `json:"destination"`
	Target      string             `json:"target"`
	SortOrder   int                `json:"sort_order"`
	IsEnabled   bool               `json:"is_enabled"`
	Children    []ResolvedMenuItem `json:"children,omitempty"`
}

type ResolverService interface {
	// ResolveBySlug finds a published page (or draft if preview token valid) by its slug
	ResolveBySlug(ctx context.Context, scope tenant.Scope, slug string, previewToken string) (ResolvedPage, error)

	// ResolveByDomain finds a published page by a custom domain
	ResolveByDomain(ctx context.Context, scope tenant.Scope, customDomain string, previewToken string) (ResolvedPage, error)
}
