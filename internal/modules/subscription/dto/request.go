package dto

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

type UpgradeSubscriptionRequest struct {
	PlanID          string  `json:"plan_id" binding:"required"`
	BillingInterval *string `json:"billing_interval"`
	Reason          string  `json:"reason"`
}

type CancelSubscriptionRequest struct {
	Reason string `json:"reason"`
}
