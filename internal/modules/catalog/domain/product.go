package domain

import (
	"encoding/json"
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

// ProductFeature adalah fitur yang melekat pada produk (hanya katalog platform).
type ProductFeature struct {
	FeatureKey   string
	Value        json.RawMessage
	DisplayLabel string
	Position     int
	Label        string // terisi oleh service (FeatureLabel); repository membiarkannya kosong
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
	IsPublic       bool
	ListingCode    string
	ListingOrder   int
	Features       []ProductFeature // urut position, lalu feature_key

	CategoryPosition int // hasil join, read-only (dipakai ListPublic)

	CreatedBy string
	UpdatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}
