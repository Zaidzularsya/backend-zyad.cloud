package dto

import (
	subscriptiondto "zyad.cloud/internal/modules/subscription/dto"
)

type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
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

// CurrentPlanResponse is the tenant-facing composition of subscription status
// (owned by the subscription domain) plus usage summary (billing/entitlement
// concern), returned by the unchanged /app/billing/current-plan endpoint.
type CurrentPlanResponse struct {
	Subscription subscriptiondto.SubscriptionResponse `json:"subscription"`
	Usage        []UsageItemResponse                  `json:"usage,omitempty"`
}

// CheckoutResponse is the hosted payment page created for an open invoice.
type CheckoutResponse struct {
	PaymentURL string  `json:"payment_url"`
	Provider   string  `json:"provider"`
	ExpiresAt  *string `json:"expires_at,omitempty"`
}

// CheckoutStatusResponse is the reconciled payment state of an invoice after
// actively querying the provider's check-status API.
type CheckoutStatusResponse struct {
	InvoiceID         string `json:"invoice_id"`
	InvoiceStatus     string `json:"invoice_status"`
	Paid              bool   `json:"paid"`
	TransactionStatus string `json:"transaction_status,omitempty"`
}
