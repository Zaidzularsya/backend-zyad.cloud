package service

import (
	"context"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type ResolvedPage struct {
	Page     domain.LandingPage
	Branding domain.LandingBranding
	Sections []domain.LandingSection
	Forms    []domain.LandingForm
	Menus    []ResolvedMenu
	Snapshot map[string]any
	IsDraft  bool
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
