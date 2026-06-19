package domain

import "time"

// DomainBinding represents the mapping between an organization domain and a landing page.
type DomainBinding struct {
	ID                   string
	OrganizationID       string
	OrganizationDomainID string
	LandingPageID        string
	IsPrimary            bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// AvailableDomain represents a verified organization domain that can be bound to a landing page.
// This is an aggregate read model representing the organization_domains table.
type AvailableDomain struct {
	ID             string
	OrganizationID string
	Type           string
	CanonicalHost  string
	Status         string
	SSLStatus      string
}
