package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

// BindDomainParams defines the parameters for binding a domain to a landing page.
type BindDomainParams struct {
	OrganizationDomainID string
	LandingPageID        string
	IsPrimary            bool
}

type DomainRepository interface {
	// BindDomain binds a verified organization domain to a landing page.
	BindDomain(ctx context.Context, scope coretenant.Scope, params BindDomainParams) (domain.DomainBinding, error)

	// UnbindDomain removes a domain binding.
	UnbindDomain(ctx context.Context, scope coretenant.Scope, bindingID string) error

	// SetPrimaryBinding sets a specific domain binding as primary for a landing page.
	SetPrimaryBinding(ctx context.Context, scope coretenant.Scope, pageID string, bindingID string) error

	// ListBindings returns all domain bindings for a landing page.
	ListBindings(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.DomainBinding, error)

	// ListAllBindings returns every domain binding owned by the organization,
	// across all landing pages. Used to build a domain→page map without
	// requiring the caller to already know which page a domain is bound to.
	ListAllBindings(ctx context.Context, scope coretenant.Scope) ([]domain.DomainBinding, error)

	// ListAvailableDomains returns all verified organization domains that can be bound.
	ListAvailableDomains(ctx context.Context, scope coretenant.Scope) ([]domain.AvailableDomain, error)
}
