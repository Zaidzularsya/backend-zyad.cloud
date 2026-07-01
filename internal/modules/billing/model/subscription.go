package model

import "time"

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
