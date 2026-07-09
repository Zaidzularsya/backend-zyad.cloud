package dto

type PlanListQuery struct {
	Page           int    `form:"page"`
	PerPage        int    `form:"per_page"`
	Type           string `form:"type"`
	IsPublic       *bool  `form:"is_public"`
	IsActive       *bool  `form:"is_active"`
	IncludeDeleted bool   `form:"include_deleted"`
	Search         string `form:"search"`
	Sort           string `form:"sort"`
	Direction      string `form:"direction"`
}

type CreatePlanRequest struct {
	Code        string             `json:"code" binding:"required"`
	Name        string             `json:"name" binding:"required"`
	Description string             `json:"description"`
	Type        string             `json:"plan_type" binding:"required"`
	IsPublic    *bool              `json:"is_public"`
	IsActive    *bool              `json:"is_active"`
	SortOrder   int                `json:"sort_order"`
	Metadata    map[string]any     `json:"metadata"`
	Prices      []PlanPriceRequest `json:"prices"`
}

type UpdatePlanRequest struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Type        *string         `json:"plan_type"`
	IsPublic    *bool           `json:"is_public"`
	IsActive    *bool           `json:"is_active"`
	SortOrder   *int            `json:"sort_order"`
	Metadata    *map[string]any `json:"metadata"`
}

type PlanPriceRequest struct {
	BillingInterval string         `json:"billing_interval" binding:"required"`
	Currency        string         `json:"currency"`
	Amount          string         `json:"amount" binding:"required"`
	IsActive        *bool          `json:"is_active"`
	Metadata        map[string]any `json:"metadata"`
}

type UpsertPlanPriceRequest struct {
	BillingInterval string         `json:"billing_interval" binding:"required"`
	Currency        string         `json:"currency"`
	Amount          string         `json:"amount" binding:"required"`
	IsActive        *bool          `json:"is_active"`
	Metadata        map[string]any `json:"metadata"`
}

type PlanPriceListQuery struct {
	IncludeDeleted bool `form:"include_deleted"`
}

type CreatePlanPriceRequest struct {
	BillingInterval string         `json:"billing_interval" binding:"required"`
	Currency        string         `json:"currency"`
	Amount          string         `json:"amount" binding:"required"`
	IsActive        *bool          `json:"is_active"`
	Metadata        map[string]any `json:"metadata"`
}

type UpdatePlanPriceRequest struct {
	BillingInterval *string         `json:"billing_interval"`
	Currency        *string         `json:"currency"`
	Amount          *string         `json:"amount"`
	IsActive        *bool           `json:"is_active"`
	Metadata        *map[string]any `json:"metadata"`
}

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

type PlanEntitlementValueRequest struct {
	FeatureKey   string         `json:"feature_key" binding:"required"`
	ValueBool    *bool          `json:"value_bool"`
	ValueInt     *int64         `json:"value_int"`
	ValueDecimal *string        `json:"value_decimal"`
	ValueString  *string        `json:"value_string"`
	Limits       map[string]any `json:"limits"`
}

type ReplacePlanEntitlementsRequest struct {
	Entitlements []PlanEntitlementValueRequest `json:"entitlements" binding:"required"`
}
