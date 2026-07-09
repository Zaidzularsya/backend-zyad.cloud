package model

import "time"

type SubscriptionEvent struct {
	ID             string
	SubscriptionID string
	OrganizationID string
	Type           string
	OldStatus      *SubscriptionStatus
	NewStatus      *SubscriptionStatus
	ActorUserID    *string
	Metadata       map[string]any
	CreatedAt      time.Time
}
