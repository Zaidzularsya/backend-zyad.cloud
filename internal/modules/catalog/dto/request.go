package dto

import (
	"encoding/json"
	"errors"

	"zyad.cloud/internal/modules/catalog/repository"
	"zyad.cloud/internal/shared/pricing"
)

// ErrPartialPricing: atribut harga pada update harus dikirim utuh.
var ErrPartialPricing = errors.New("charge_type and payment_timing must be sent together (with billing_frequency when recurring)")

// ErrPartialListing: blok publikasi pada update dikirim utuh (tiga field) atau tidak sama sekali.
var ErrPartialListing = errors.New("is_public, listing_code, dan listing_order harus dikirim bersamaan")

// FeatureRequest adalah satu fitur produk; Value dibiarkan mentah agar tipe JSON
// (bool/angka/string) bisa divalidasi terhadap registry oleh service.
type FeatureRequest struct {
	FeatureKey   string          `json:"feature_key" binding:"required,max=100"`
	Value        json.RawMessage `json:"value" binding:"required"`
	DisplayLabel string          `json:"display_label"`
	Position     int             `json:"position"`
}

type ProductListQuery struct {
	Q          string `form:"q"`
	CategoryID string `form:"category_id"`
	IsActive   string `form:"is_active" binding:"omitempty,oneof=true false"`
	Page       int    `form:"page"`
	PerPage    int    `form:"per_page"`
}

type CreateProductRequest struct {
	CategoryID  string `json:"category_id"`
	SKU         string `json:"sku" binding:"max=64"`
	Name        string `json:"name" binding:"required,max=200"`
	Description string `json:"description" binding:"max=5000"`
	Unit        string `json:"unit" binding:"max=30"`
	BasePrice   string `json:"base_price"`
	TaxPercent  string `json:"tax_percent"`

	ChargeType       string `json:"charge_type" binding:"omitempty,oneof=one_time recurring"`
	BillingFrequency string `json:"billing_frequency"`
	PaymentTiming    string `json:"payment_timing" binding:"omitempty,oneof=prepaid postpaid"`

	IsActive *bool `json:"is_active"`

	IsPublic     bool             `json:"is_public"`
	ListingCode  string           `json:"listing_code"`
	ListingOrder int              `json:"listing_order"`
	Features     []FeatureRequest `json:"features" binding:"omitempty,dive"`
}

type UpdateProductRequest struct {
	CategoryID  *string `json:"category_id"`
	SKU         *string `json:"sku" binding:"omitempty,max=64"`
	Name        *string `json:"name" binding:"omitempty,max=200"`
	Description *string `json:"description" binding:"omitempty,max=5000"`
	Unit        *string `json:"unit" binding:"omitempty,max=30"`
	BasePrice   *string `json:"base_price"`
	TaxPercent  *string `json:"tax_percent"`

	ChargeType       *string `json:"charge_type" binding:"omitempty,oneof=one_time recurring"`
	BillingFrequency *string `json:"billing_frequency"`
	PaymentTiming    *string `json:"payment_timing" binding:"omitempty,oneof=prepaid postpaid"`

	IsActive *bool `json:"is_active"`

	IsPublic     *bool             `json:"is_public"`
	ListingCode  *string           `json:"listing_code"`
	ListingOrder *int              `json:"listing_order"`
	Features     *[]FeatureRequest `json:"features" binding:"omitempty,dive"` // nil = tidak diubah; [] = hapus semua
}

// Listing mengembalikan blok publikasi utuh, atau nil bila tidak ada yang dikirim.
func (r UpdateProductRequest) Listing() (*repository.ListingParams, error) {
	if r.IsPublic == nil && r.ListingCode == nil && r.ListingOrder == nil {
		return nil, nil
	}
	if r.IsPublic == nil || r.ListingCode == nil || r.ListingOrder == nil {
		return nil, ErrPartialListing
	}
	return &repository.ListingParams{IsPublic: *r.IsPublic, ListingCode: *r.ListingCode, ListingOrder: *r.ListingOrder}, nil
}

// FeatureValues memetakan request fitur ke parameter repository; nil bila field tidak dikirim.
func FeatureValues(in []FeatureRequest) []repository.FeatureValue {
	out := make([]repository.FeatureValue, 0, len(in))
	for _, f := range in {
		out = append(out, repository.FeatureValue{FeatureKey: f.FeatureKey, Value: f.Value, DisplayLabel: f.DisplayLabel, Position: f.Position})
	}
	return out
}

// Pricing mengembalikan atribut harga utuh, atau nil bila tidak ada yang
// dikirim. Mengirim sebagian saja (tanpa charge_type/payment_timing, atau
// billing_frequency tanpa charge_type) ditolak agar tidak menimpa diam-diam.
func (r UpdateProductRequest) Pricing() (*pricing.Attributes, error) {
	if r.ChargeType == nil && r.BillingFrequency == nil && r.PaymentTiming == nil {
		return nil, nil
	}
	if r.ChargeType == nil || r.PaymentTiming == nil {
		return nil, ErrPartialPricing
	}
	a := pricing.Attributes{ChargeType: pricing.ChargeType(*r.ChargeType), PaymentTiming: pricing.PaymentTiming(*r.PaymentTiming)}
	if r.BillingFrequency != nil {
		a.Frequency = pricing.Frequency(*r.BillingFrequency)
	}
	return &a, nil
}

type CategoryRequest struct {
	Name     *string `json:"name" binding:"omitempty,max=100"`
	Position *int    `json:"position"`
}
