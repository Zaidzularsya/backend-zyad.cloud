package domain

import "time"

// TenantSettings is the social_tenant_settings row of one organization.
type TenantSettings struct {
	OrganizationID string
	// CustomerClaimAutoOffer: offer this tenant's CRM contacts to users whose
	// verified email matches. Default true when the row is absent.
	CustomerClaimAutoOffer bool
	UpdatedBy              string
	// UpdatedAt is nil while the tenant still runs on defaults.
	UpdatedAt *time.Time
}

// DefaultTenantSettings is what an organization without a settings row gets.
func DefaultTenantSettings(organizationID string) TenantSettings {
	return TenantSettings{OrganizationID: organizationID, CustomerClaimAutoOffer: true}
}
