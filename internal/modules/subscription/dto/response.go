package dto

import (
	productdto "zyad.cloud/internal/modules/product/dto"
)

type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type SubscriptionResponse struct {
	ID                 string                   `json:"id"`
	OrganizationID     string                   `json:"organization_id"`
	PlanID             string                   `json:"plan_id"`
	Plan               *productdto.PlanResponse `json:"plan,omitempty"`
	Status             string                   `json:"status"`
	BillingInterval    string                   `json:"billing_interval"`
	CurrentPeriodStart *string                  `json:"current_period_start,omitempty"`
	CurrentPeriodEnd   *string                  `json:"current_period_end,omitempty"`
	TrialStart         *string                  `json:"trial_start,omitempty"`
	TrialEnd           *string                  `json:"trial_end,omitempty"`
	CancelAtPeriodEnd  bool                     `json:"cancel_at_period_end"`
	CanceledAt         *string                  `json:"canceled_at,omitempty"`
	SuspendedAt        *string                  `json:"suspended_at,omitempty"`
	Metadata           map[string]any           `json:"metadata"`
	CreatedAt          string                   `json:"created_at"`
	UpdatedAt          string                   `json:"updated_at"`
}

type SubscriptionListResponse struct {
	Items []SubscriptionResponse `json:"items"`
	Meta  PaginationMeta         `json:"meta"`
}
