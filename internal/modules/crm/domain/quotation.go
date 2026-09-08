package domain

import "time"

type QuotationStatus string

const (
	QuotationStatusDraft    QuotationStatus = "draft"
	QuotationStatusSent     QuotationStatus = "sent"
	QuotationStatusApproved QuotationStatus = "approved"
	QuotationStatusRejected QuotationStatus = "rejected"
	QuotationStatusExpired  QuotationStatus = "expired"
)

func (s QuotationStatus) IsValid() bool {
	switch s {
	case QuotationStatusDraft, QuotationStatusSent, QuotationStatusApproved, QuotationStatusRejected, QuotationStatusExpired:
		return true
	default:
		return false
	}
}

// QuotationItem is immutable once its quotation is created — Fase 4 does not
// support editing line items after creation (only header fields and status
// transitions), see docs/reference-crm.md "Fase Implementasi" for the
// documented limitation.
type QuotationItem struct {
	ID              string
	Description     string
	Quantity        string
	UnitPrice       string
	DiscountPercent *string
	LineTotal       string
	Position        int
}

// Quotation money fields (Subtotal/DiscountTotal/TaxTotal/GrandTotal) are
// decimal strings, computed once at creation time by
// QuotationService.computeTotals from Items — see that method's doc comment
// for the simplification this implies (float64 arithmetic, not a decimal
// library) versus the ::text-cast-only convention used elsewhere in this
// module for stored values.
type Quotation struct {
	ID              string
	OrganizationID  string
	DealID          *string
	ContactID       *string
	CompanyID       *string
	QuotationNumber string
	Status          QuotationStatus
	ValidUntil      *time.Time
	Subtotal        string
	DiscountTotal   string
	TaxTotal        string
	GrandTotal      string
	Currency        string
	Notes           string
	SentAt          *time.Time
	ApprovedAt      *time.Time
	RejectedAt      *time.Time
	Items           []QuotationItem
	CreatedBy       string
	UpdatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}
