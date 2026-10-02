package dto

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
	IsActive    *bool  `json:"is_active"`
}

type UpdateProductRequest struct {
	CategoryID  *string `json:"category_id"`
	SKU         *string `json:"sku" binding:"omitempty,max=64"`
	Name        *string `json:"name" binding:"omitempty,max=200"`
	Description *string `json:"description" binding:"omitempty,max=5000"`
	Unit        *string `json:"unit" binding:"omitempty,max=30"`
	BasePrice   *string `json:"base_price"`
	TaxPercent  *string `json:"tax_percent"`
	IsActive    *bool   `json:"is_active"`
}

type CategoryRequest struct {
	Name     *string `json:"name" binding:"omitempty,max=100"`
	Position *int    `json:"position"`
}
