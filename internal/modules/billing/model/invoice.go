package model

import "time"

type InvoiceStatus string

const (
	InvoiceStatusDraft   InvoiceStatus = "draft"
	InvoiceStatusOpen    InvoiceStatus = "open"
	InvoiceStatusPaid    InvoiceStatus = "paid"
	InvoiceStatusVoid    InvoiceStatus = "void"
	InvoiceStatusExpired InvoiceStatus = "expired"
	InvoiceStatusFailed  InvoiceStatus = "failed"
)

func (s InvoiceStatus) IsValid() bool {
	switch s {
	case InvoiceStatusDraft,
		InvoiceStatusOpen,
		InvoiceStatusPaid,
		InvoiceStatusVoid,
		InvoiceStatusExpired,
		InvoiceStatusFailed:
		return true
	default:
		return false
	}
}

type InvoiceItemType string

const (
	InvoiceItemTypeSubscription InvoiceItemType = "subscription"
	InvoiceItemTypeAddon        InvoiceItemType = "addon"
	InvoiceItemTypeAdjustment   InvoiceItemType = "adjustment"
	InvoiceItemTypeTax          InvoiceItemType = "tax"
	InvoiceItemTypeDiscount     InvoiceItemType = "discount"
)

func (t InvoiceItemType) IsValid() bool {
	switch t {
	case InvoiceItemTypeSubscription,
		InvoiceItemTypeAddon,
		InvoiceItemTypeAdjustment,
		InvoiceItemTypeTax,
		InvoiceItemTypeDiscount:
		return true
	default:
		return false
	}
}

type Invoice struct {
	ID             string
	OrganizationID string
	SubscriptionID *string
	InvoiceNumber  string
	Status         InvoiceStatus
	Currency       string
	SubtotalAmount string
	DiscountAmount string
	TaxAmount      string
	TotalAmount    string
	DueDate        *time.Time
	PaidAt         *time.Time
	Metadata       map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (i Invoice) IsPaid() bool {
	return i.Status == InvoiceStatusPaid && i.PaidAt != nil
}

type InvoiceItem struct {
	ID          string
	InvoiceID   string
	Type        InvoiceItemType
	Description string
	Quantity    string
	UnitAmount  string
	TotalAmount string
	Metadata    map[string]any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
