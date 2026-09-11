package domain

import "time"

// LandingPricingPlan is a tenant-authored pricing card shown by the
// `zyad-pricing-plans` GrapesJS block (sentinel `data-zyad-slot="pricing-plans"`).
// It is marketing content the tenant writes themselves — not an integration
// with the platform's own subscription plans (see internal/modules/product).
type LandingPricingPlan struct {
	ID             string
	OrganizationID string
	Name           string
	PriceLabel     string
	IntervalLabel  string
	Description    string
	Features       []string
	CTALabel       string
	CTAURL         string
	IsFeatured     bool
	SortOrder      int
	IsEnabled      bool
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}
