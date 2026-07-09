package dto

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
