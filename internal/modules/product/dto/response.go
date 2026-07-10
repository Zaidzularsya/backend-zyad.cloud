package dto

type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type PlanResponse struct {
	ID          string              `json:"id"`
	Code        string              `json:"code"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	PlanType    string              `json:"plan_type"`
	IsPublic    bool                `json:"is_public"`
	IsActive    bool                `json:"is_active"`
	SortOrder   int                 `json:"sort_order"`
	Metadata    map[string]any      `json:"metadata"`
	Prices      []PlanPriceResponse `json:"prices,omitempty"`
	CreatedAt   string              `json:"created_at"`
	UpdatedAt   string              `json:"updated_at"`
	DeletedAt   *string             `json:"deleted_at,omitempty"`
}

type PlanListResponse struct {
	Items []PlanResponse `json:"items"`
	Meta  PaginationMeta `json:"meta"`
}

type PublicPlanPriceSummary struct {
	BillingInterval string `json:"billing_interval"`
	Currency        string `json:"currency"`
	Amount          string `json:"amount"`
}

type PublicPlanBenefit struct {
	Label string `json:"label"`
	Value string `json:"value,omitempty"`
}

type PublicPlanSummary struct {
	ID          string                   `json:"id"`
	Code        string                   `json:"code"`
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	SortOrder   int                      `json:"sort_order"`
	Prices      []PublicPlanPriceSummary `json:"prices"`
	Benefits    []PublicPlanBenefit      `json:"benefits"`
}

type PublicPlanListResponse struct {
	Items []PublicPlanSummary `json:"items"`
}

type PlanPriceResponse struct {
	ID              string         `json:"id"`
	PlanID          string         `json:"plan_id"`
	BillingInterval string         `json:"billing_interval"`
	Currency        string         `json:"currency"`
	Amount          string         `json:"amount"`
	IsActive        bool           `json:"is_active"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
	DeletedAt       *string        `json:"deleted_at,omitempty"`
}

type FeatureResponse struct {
	ID            string `json:"id"`
	FeatureKey    string `json:"feature_key"`
	Module        string `json:"module"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	ValueType     string `json:"value_type"`
	Unit          string `json:"unit,omitempty"`
	ResetStrategy string `json:"reset_strategy"`
	IsActive      bool   `json:"is_active"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type FeatureListResponse struct {
	Items []FeatureResponse `json:"items"`
	Meta  PaginationMeta    `json:"meta"`
}

type PlanEntitlementResponse struct {
	ID           string         `json:"id"`
	PlanID       string         `json:"plan_id"`
	FeatureID    string         `json:"feature_id"`
	FeatureKey   string         `json:"feature_key"`
	ValueBool    *bool          `json:"value_bool,omitempty"`
	ValueInt     *int64         `json:"value_int,omitempty"`
	ValueDecimal *string        `json:"value_decimal,omitempty"`
	ValueString  *string        `json:"value_string,omitempty"`
	Limits       map[string]any `json:"limits"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

type PlanEntitlementListResponse struct {
	Items []PlanEntitlementResponse `json:"items"`
}
