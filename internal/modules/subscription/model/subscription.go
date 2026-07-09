package model

import (
	"time"

	productmodel "zyad.cloud/internal/modules/product/model"
)

type SubscriptionStatus string

const (
	SubscriptionStatusTrialing    SubscriptionStatus = "trialing"
	SubscriptionStatusActive      SubscriptionStatus = "active"
	SubscriptionStatusPastDue     SubscriptionStatus = "past_due"
	SubscriptionStatusGracePeriod SubscriptionStatus = "grace_period"
	SubscriptionStatusSuspended   SubscriptionStatus = "suspended"
	SubscriptionStatusCanceled    SubscriptionStatus = "canceled"
	SubscriptionStatusExpired     SubscriptionStatus = "expired"
)

func (s SubscriptionStatus) IsValid() bool {
	switch s {
	case SubscriptionStatusTrialing,
		SubscriptionStatusActive,
		SubscriptionStatusPastDue,
		SubscriptionStatusGracePeriod,
		SubscriptionStatusSuspended,
		SubscriptionStatusCanceled,
		SubscriptionStatusExpired:
		return true
	default:
		return false
	}
}

func (s SubscriptionStatus) IsUsable() bool {
	switch s {
	case SubscriptionStatusTrialing,
		SubscriptionStatusActive,
		SubscriptionStatusPastDue,
		SubscriptionStatusGracePeriod:
		return true
	default:
		return false
	}
}

// BillingInterval is shared with the product/catalog domain since plan prices
// and subscriptions both express recurring intervals the same way.
type BillingInterval = productmodel.BillingInterval

const (
	BillingIntervalMonthly = productmodel.BillingIntervalMonthly
	BillingIntervalYearly  = productmodel.BillingIntervalYearly
	BillingIntervalOneTime = productmodel.BillingIntervalOneTime
	BillingIntervalCustom  = productmodel.BillingIntervalCustom
)

type Subscription struct {
	ID                 string
	OrganizationID     string
	PlanID             string
	Status             SubscriptionStatus
	BillingInterval    BillingInterval
	CurrentPeriodStart *time.Time
	CurrentPeriodEnd   *time.Time
	TrialStart         *time.Time
	TrialEnd           *time.Time
	CancelAtPeriodEnd  bool
	CanceledAt         *time.Time
	SuspendedAt        *time.Time
	Metadata           map[string]any
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (s Subscription) IsUsable() bool {
	return s.Status.IsUsable()
}
