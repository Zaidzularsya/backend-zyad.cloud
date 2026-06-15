package model

import "time"

type EntitlementSource string

const (
	EntitlementSourcePlan             EntitlementSource = "plan"
	EntitlementSourceAddon            EntitlementSource = "addon"
	EntitlementSourceTrial            EntitlementSource = "trial"
	EntitlementSourcePlatformOverride EntitlementSource = "platform_override"
)

func (s EntitlementSource) IsValid() bool {
	switch s {
	case EntitlementSourcePlan,
		EntitlementSourceAddon,
		EntitlementSourceTrial,
		EntitlementSourcePlatformOverride:
		return true
	default:
		return false
	}
}

type EntitlementStatus string

const (
	EntitlementStatusActive   EntitlementStatus = "active"
	EntitlementStatusInactive EntitlementStatus = "inactive"
	EntitlementStatusExpired  EntitlementStatus = "expired"
	EntitlementStatusRevoked  EntitlementStatus = "revoked"
)

func (s EntitlementStatus) IsValid() bool {
	switch s {
	case EntitlementStatusActive,
		EntitlementStatusInactive,
		EntitlementStatusExpired,
		EntitlementStatusRevoked:
		return true
	default:
		return false
	}
}

type Entitlement struct {
	ID              string
	OrganizationID  string
	FeatureKey      string
	Source          EntitlementSource
	SourceReference string
	Status          EntitlementStatus
	Limits          map[string]any
	Version         int64
	EffectiveFrom   time.Time
	EffectiveUntil  *time.Time
	Reason          string
	CreatedBy       string
	UpdatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type UsageCounter struct {
	ID             string
	OrganizationID string
	FeatureKey     string
	MetricKey      string
	PeriodStart    time.Time
	PeriodEnd      time.Time
	UsageValue     int64
	Version        int64
	LastRecordedAt *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (e Entitlement) IsEffective(now time.Time) bool {
	if e.Status != EntitlementStatusActive || now.Before(e.EffectiveFrom) {
		return false
	}
	return e.EffectiveUntil == nil || now.Before(*e.EffectiveUntil)
}

func (c UsageCounter) Contains(at time.Time) bool {
	return !at.Before(c.PeriodStart) && at.Before(c.PeriodEnd)
}
