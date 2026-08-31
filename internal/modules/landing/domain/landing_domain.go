package domain

import "time"

// DomainBinding represents the mapping between an organization domain and a landing page.
type DomainBinding struct {
	ID                   string    `json:"id"`
	OrganizationID       string    `json:"organization_id"`
	OrganizationDomainID string    `json:"organization_domain_id"`
	LandingPageID        string    `json:"landing_page_id"`
	IsPrimary            bool      `json:"is_primary"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// AvailableDomain represents a verified organization domain that can be bound to a landing page.
// This is an aggregate read model representing the organization_domains table.
type AvailableDomain struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Type           string `json:"type"`
	CanonicalHost  string `json:"canonical_host"`
	Status         string `json:"status"`
	SSLStatus      string `json:"ssl_status"`
}
