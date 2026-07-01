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

type SubscriptionListQuery struct {
	Page           int    `form:"page"`
	PerPage        int    `form:"per_page"`
	OrganizationID string `form:"organization_id"`
	PlanID         string `form:"plan_id"`
	Status         string `form:"status"`
}

type CreateSubscriptionRequest struct {
	OrganizationID     string         `json:"organization_id" binding:"required"`
	PlanID             string         `json:"plan_id" binding:"required"`
	BillingInterval    string         `json:"billing_interval" binding:"required"`
	Status             string         `json:"status"`
	CurrentPeriodStart *string        `json:"current_period_start"`
	CurrentPeriodEnd   *string        `json:"current_period_end"`
	TrialStart         *string        `json:"trial_start"`
	TrialEnd           *string        `json:"trial_end"`
	CancelAtPeriodEnd  bool           `json:"cancel_at_period_end"`
	Metadata           map[string]any `json:"metadata"`
}

type UpdateSubscriptionRequest struct {
	PlanID             *string         `json:"plan_id"`
	Status             *string         `json:"status"`
	BillingInterval    *string         `json:"billing_interval"`
	CurrentPeriodStart *string         `json:"current_period_start"`
	CurrentPeriodEnd   *string         `json:"current_period_end"`
	TrialStart         *string         `json:"trial_start"`
	TrialEnd           *string         `json:"trial_end"`
	CancelAtPeriodEnd  *bool           `json:"cancel_at_period_end"`
	Metadata           *map[string]any `json:"metadata"`
	Reason             string          `json:"reason"`
}

type InvoiceListQuery struct {
	Page           int    `form:"page"`
	PerPage        int    `form:"per_page"`
	OrganizationID string `form:"organization_id"`
	SubscriptionID string `form:"subscription_id"`
	Status         string `form:"status"`
}

type CreateInvoiceRequest struct {
	OrganizationID string                     `json:"organization_id" binding:"required"`
	SubscriptionID string                     `json:"subscription_id"`
	Currency       string                     `json:"currency"`
	DueDate        *string                    `json:"due_date"`
	Items          []CreateInvoiceItemRequest `json:"items" binding:"required"`
	Metadata       map[string]any             `json:"metadata"`
}

type CreateInvoiceItemRequest struct {
	Type        string         `json:"item_type" binding:"required"`
	Description string         `json:"description" binding:"required"`
	Quantity    string         `json:"quantity"`
	UnitAmount  string         `json:"unit_amount" binding:"required"`
	Metadata    map[string]any `json:"metadata"`
}

type MarkInvoicePaidRequest struct {
	Provider          string         `json:"provider"`
	ProviderReference string         `json:"provider_reference"`
	PaymentMethod     string         `json:"payment_method"`
	Amount            string         `json:"amount"`
	Currency          string         `json:"currency"`
	PaidAt            *string        `json:"paid_at"`
	RawPayload        map[string]any `json:"raw_payload"`
}

type RecordPaymentProviderEventRequest struct {
	PaymentID       string         `json:"payment_id"`
	InvoiceID       string         `json:"invoice_id"`
	Provider        string         `json:"provider" binding:"required"`
	EventType       string         `json:"event_type" binding:"required"`
	ProviderEventID string         `json:"provider_event_id" binding:"required"`
	ProcessedAt     *string        `json:"processed_at"`
	Payload         map[string]any `json:"payload"`
}

type UsageQuery struct {
	FeatureKey  string `form:"feature_key" binding:"required"`
	MetricKey   string `form:"metric_key" binding:"required"`
	LimitKey    string `form:"limit_key" binding:"required"`
	PeriodStart string `form:"period_start" binding:"required"`
	PeriodEnd   string `form:"period_end" binding:"required"`
}

type UpgradeSubscriptionRequest struct {
	PlanID          string  `json:"plan_id" binding:"required"`
	BillingInterval *string `json:"billing_interval"`
	Reason          string  `json:"reason"`
}

type CancelSubscriptionRequest struct {
	Reason string `json:"reason"`
}
