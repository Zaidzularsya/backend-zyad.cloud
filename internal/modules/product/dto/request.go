package dto

type FeatureListQuery struct {
	Page     int    `form:"page"`
	PerPage  int    `form:"per_page"`
	Module   string `form:"module"`
	IsActive *bool  `form:"is_active"`
	Search   string `form:"search"`
}

type CreateFeatureRequest struct {
	FeatureKey    string `json:"feature_key" binding:"required"`
	Module        string `json:"module" binding:"required"`
	Name          string `json:"name" binding:"required"`
	Description   string `json:"description"`
	ValueType     string `json:"value_type" binding:"required"`
	Unit          string `json:"unit"`
	ResetStrategy string `json:"reset_strategy"`
	IsActive      *bool  `json:"is_active"`
}

type UpdateFeatureRequest struct {
	Module        *string `json:"module"`
	Name          *string `json:"name"`
	Description   *string `json:"description"`
	ValueType     *string `json:"value_type"`
	Unit          *string `json:"unit"`
	ResetStrategy *string `json:"reset_strategy"`
	IsActive      *bool   `json:"is_active"`
}
