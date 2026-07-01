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

type PaymentEvent struct {
	ID              string
	PaymentID       *string
	InvoiceID       *string
	Provider        PaymentProvider
	Type            string
	ProviderEventID string
	Payload         map[string]any
	ProcessedAt     *time.Time
	CreatedAt       time.Time
}
