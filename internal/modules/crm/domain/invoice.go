package domain

import "time"

type InvoiceStatus string

const (
	InvoiceStatusDraft     InvoiceStatus = "draft"
	InvoiceStatusSent      InvoiceStatus = "sent"
	InvoiceStatusPaid      InvoiceStatus = "paid"
	InvoiceStatusOverdue   InvoiceStatus = "overdue"
	InvoiceStatusCancelled InvoiceStatus = "cancelled"
)

func (s InvoiceStatus) IsValid() bool {
	switch s {
	case InvoiceStatusDraft, InvoiceStatusSent, InvoiceStatusPaid, InvoiceStatusOverdue, InvoiceStatusCancelled:
		return true
	default:
		return false
	}
}

// InvoiceItem is immutable once its invoice is created — same Fase 4
// limitation as QuotationItem.
type InvoiceItem struct {
	ID              string
	Description     string
	Quantity        string
	UnitPrice       string
	DiscountPercent *string
	LineTotal       string
	Position        int
}

// crm_invoices is tenant-to-their-own-customer invoicing — NOT
// billing_invoices (platform-to-tenant SaaS subscription billing). See
// docs/reference-crm.md "Naming Conflict".
type Invoice struct {
	ID             string
	OrganizationID string
	QuotationID    *string
	DealID         *string
	ContactID      *string
	CompanyID      *string
	InvoiceNumber  string
	Status         InvoiceStatus
	IssueDate      *time.Time
	DueDate        *time.Time
	Subtotal       string
	TaxTotal       string
	GrandTotal     string
	AmountPaid     string
	PaidAt         *time.Time
	Currency       string
	Items          []InvoiceItem
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}
