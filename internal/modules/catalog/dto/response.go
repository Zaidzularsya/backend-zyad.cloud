package dto

import (
	"time"

	"zyad.cloud/internal/modules/catalog/domain"
)

type ProductResponse struct {
	ID               string    `json:"id"`
	CategoryID       *string   `json:"category_id"`
	CategoryName     string    `json:"category_name,omitempty"`
	SKU              string    `json:"sku,omitempty"`
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	Unit             string    `json:"unit"`
	BasePrice        string    `json:"base_price"`
	TaxPercent       string    `json:"tax_percent"`
	Currency         string    `json:"currency"`
	ChargeType       string    `json:"charge_type"`
	BillingFrequency *string   `json:"billing_frequency"`
	PaymentTiming    string    `json:"payment_timing"`
	IsActive         bool      `json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func ProductFromDomain(p domain.Product) ProductResponse {
	var freq *string
	if p.Pricing.Frequency != "" {
		f := string(p.Pricing.Frequency)
		freq = &f
	}
	return ProductResponse{ChargeType: string(p.Pricing.ChargeType), BillingFrequency: freq, PaymentTiming: string(p.Pricing.PaymentTiming), ID: p.ID, CategoryID: p.CategoryID, CategoryName: p.CategoryName, SKU: p.SKU, Name: p.Name,
		Description: p.Description, Unit: p.Unit, BasePrice: p.BasePrice, TaxPercent: p.TaxPercent,
		Currency: p.Currency, IsActive: p.IsActive, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}

type CategoryResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

func CategoryFromDomain(c domain.Category) CategoryResponse {
	return CategoryResponse{ID: c.ID, Name: c.Name, Position: c.Position}
}

type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

func BuildMeta(page, perPage int, total int64) PaginationMeta {
	totalPages := 0
	if perPage > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}
	return PaginationMeta{Page: page, PerPage: perPage, Total: total, TotalPages: totalPages}
}
