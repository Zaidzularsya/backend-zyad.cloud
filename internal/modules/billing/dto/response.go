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

type SubscriptionResponse struct {
	ID                 string         `json:"id"`
	OrganizationID     string         `json:"organization_id"`
	PlanID             string         `json:"plan_id"`
	Plan               *PlanResponse  `json:"plan,omitempty"`
	Status             string         `json:"status"`
	BillingInterval    string         `json:"billing_interval"`
	CurrentPeriodStart *string        `json:"current_period_start,omitempty"`
	CurrentPeriodEnd   *string        `json:"current_period_end,omitempty"`
	TrialStart         *string        `json:"trial_start,omitempty"`
	TrialEnd           *string        `json:"trial_end,omitempty"`
	CancelAtPeriodEnd  bool           `json:"cancel_at_period_end"`
	CanceledAt         *string        `json:"canceled_at,omitempty"`
	SuspendedAt        *string        `json:"suspended_at,omitempty"`
	Metadata           map[string]any `json:"metadata"`
	CreatedAt          string         `json:"created_at"`
	UpdatedAt          string         `json:"updated_at"`
}

type SubscriptionListResponse struct {
	Items []SubscriptionResponse `json:"items"`
	Meta  PaginationMeta         `json:"meta"`
}

type InvoiceResponse struct {
	ID             string                `json:"id"`
	OrganizationID string                `json:"organization_id"`
	SubscriptionID *string               `json:"subscription_id,omitempty"`
	InvoiceNumber  string                `json:"invoice_number"`
	Status         string                `json:"status"`
	Currency       string                `json:"currency"`
	SubtotalAmount string                `json:"subtotal_amount"`
	DiscountAmount string                `json:"discount_amount"`
	TaxAmount      string                `json:"tax_amount"`
	TotalAmount    string                `json:"total_amount"`
	DueDate        *string               `json:"due_date,omitempty"`
	PaidAt         *string               `json:"paid_at,omitempty"`
	Items          []InvoiceItemResponse `json:"items,omitempty"`
	Metadata       map[string]any        `json:"metadata"`
	CreatedAt      string                `json:"created_at"`
	UpdatedAt      string                `json:"updated_at"`
}

type InvoiceListResponse struct {
	Items []InvoiceResponse `json:"items"`
	Meta  PaginationMeta    `json:"meta"`
}

type InvoiceItemResponse struct {
	ID          string         `json:"id"`
	InvoiceID   string         `json:"invoice_id"`
	ItemType    string         `json:"item_type"`
	Description string         `json:"description"`
	Quantity    string         `json:"quantity"`
	UnitAmount  string         `json:"unit_amount"`
	TotalAmount string         `json:"total_amount"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

type PaymentResponse struct {
	ID                string         `json:"id"`
	InvoiceID         string         `json:"invoice_id"`
	OrganizationID    string         `json:"organization_id"`
	Provider          string         `json:"provider"`
	ProviderReference string         `json:"provider_reference,omitempty"`
	PaymentMethod     string         `json:"payment_method,omitempty"`
	Status            string         `json:"status"`
	Amount            string         `json:"amount"`
	Currency          string         `json:"currency"`
	PaidAt            *string        `json:"paid_at,omitempty"`
	RawPayload        map[string]any `json:"raw_payload,omitempty"`
	CreatedAt         string         `json:"created_at"`
	UpdatedAt         string         `json:"updated_at"`
}

type PaymentEventResponse struct {
	ID              string         `json:"id"`
	PaymentID       *string        `json:"payment_id,omitempty"`
	InvoiceID       *string        `json:"invoice_id,omitempty"`
	Provider        string         `json:"provider"`
	EventType       string         `json:"event_type"`
	ProviderEventID string         `json:"provider_event_id,omitempty"`
	Payload         map[string]any `json:"payload"`
	ProcessedAt     *string        `json:"processed_at,omitempty"`
	CreatedAt       string         `json:"created_at"`
}

type UsageItemResponse struct {
	FeatureKey     string  `json:"feature_key"`
	MetricKey      string  `json:"metric_key,omitempty"`
	UsedValue      string  `json:"used_value"`
	LimitValue     *string `json:"limit_value,omitempty"`
	RemainingValue *string `json:"remaining_value,omitempty"`
	PeriodStart    *string `json:"period_start,omitempty"`
	PeriodEnd      *string `json:"period_end,omitempty"`
}

type CurrentPlanResponse struct {
	Subscription SubscriptionResponse `json:"subscription"`
	Usage        []UsageItemResponse  `json:"usage,omitempty"`
}
