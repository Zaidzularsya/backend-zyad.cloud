package model

import "time"

type PaymentProvider string

const (
	PaymentProviderManual   PaymentProvider = "manual"
	PaymentProviderXendit   PaymentProvider = "xendit"
	PaymentProviderMidtrans PaymentProvider = "midtrans"
	PaymentProviderDoku     PaymentProvider = "doku"
)

func (p PaymentProvider) IsValid() bool {
	switch p {
	case PaymentProviderManual,
		PaymentProviderXendit,
		PaymentProviderMidtrans,
		PaymentProviderDoku:
		return true
	default:
		return false
	}
}

type PaymentStatus string

const (
	PaymentStatusPending  PaymentStatus = "pending"
	PaymentStatusPaid     PaymentStatus = "paid"
	PaymentStatusFailed   PaymentStatus = "failed"
	PaymentStatusExpired  PaymentStatus = "expired"
	PaymentStatusRefunded PaymentStatus = "refunded"
)

func (s PaymentStatus) IsValid() bool {
	switch s {
	case PaymentStatusPending,
		PaymentStatusPaid,
		PaymentStatusFailed,
		PaymentStatusExpired,
		PaymentStatusRefunded:
		return true
	default:
		return false
	}
}

type Payment struct {
	ID                string
	InvoiceID         string
	OrganizationID    string
	Provider          PaymentProvider
	ProviderReference string
	PaymentMethod     string
	Status            PaymentStatus
	Amount            string
	Currency          string
	PaidAt            *time.Time
	RawPayload        map[string]any
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (p Payment) IsPaid() bool {
	return p.Status == PaymentStatusPaid && p.PaidAt != nil
}
