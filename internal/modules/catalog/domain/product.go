package domain

import (
	"time"

	"zyad.cloud/internal/shared/pricing"
)

// Category mengelompokkan produk katalog tenant.
type Category struct {
	ID             string
	OrganizationID string
	Name           string
	Position       int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Product adalah item katalog tenant. Nilai uang & persen disimpan sebagai
// string desimal agar tidak melewati float64.
type Product struct {
	ID             string
	OrganizationID string
	CategoryID     *string
	CategoryName   string // hasil join, read-only
	SKU            string
	Name           string
	Description    string
	Unit           string
	BasePrice      string
	TaxPercent     string
	Currency       string
	Pricing        pricing.Attributes
	IsActive       bool
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}
