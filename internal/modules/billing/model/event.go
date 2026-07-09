package model

import "time"

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
