package model

import "time"

type PlanType string

const (
	PlanTypeFree       PlanType = "free"
	PlanTypeTrial      PlanType = "trial"
	PlanTypePaid       PlanType = "paid"
	PlanTypeEnterprise PlanType = "enterprise"
)

func (t PlanType) IsValid() bool {
	switch t {
	case PlanTypeFree,
		PlanTypeTrial,
		PlanTypePaid,
		PlanTypeEnterprise:
		return true
	default:
		return false
	}
}

type BillingInterval string

const (
	BillingIntervalMonthly BillingInterval = "monthly"
	BillingIntervalYearly  BillingInterval = "yearly"
	BillingIntervalOneTime BillingInterval = "one_time"
	BillingIntervalCustom  BillingInterval = "custom"
)

func (i BillingInterval) IsValidPlanPriceInterval() bool {
	switch i {
	case BillingIntervalMonthly,
		BillingIntervalYearly,
		BillingIntervalOneTime,
		BillingIntervalCustom:
		return true
	default:
		return false
	}
}

func (i BillingInterval) IsValidSubscriptionInterval() bool {
	switch i {
	case BillingIntervalMonthly,
		BillingIntervalYearly,
		BillingIntervalCustom:
		return true
	default:
		return false
	}
}

type Plan struct {
	ID          string
	Code        string
	Name        string
	Description string
	Type        PlanType
	IsPublic    bool
	IsActive    bool
	SortOrder   int
	Metadata    map[string]any
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

func (p Plan) IsAvailable() bool {
	return p.DeletedAt == nil && p.IsActive
}

type PlanPrice struct {
	ID              string
	PlanID          string
	BillingInterval BillingInterval
	Currency        string
	Amount          string
	IsActive        bool
	Metadata        map[string]any
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

func (p PlanPrice) IsAvailable() bool {
	return p.DeletedAt == nil && p.IsActive
}
