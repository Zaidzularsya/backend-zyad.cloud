package model

import "time"

type PartnerType string

const (
	PartnerTypeCustomer PartnerType = "customer"
	PartnerTypeVendor   PartnerType = "vendor"
)

func (t PartnerType) IsValid() bool {
	return t == PartnerTypeCustomer || t == PartnerTypeVendor
}

type BusinessPartner struct {
	ID                 string
	PartnerType        PartnerType
	Code               string
	Name               string
	TaxID              string
	Address            string
	ContactInfo        map[string]any
	ControlAccountID   string
	ControlAccountCode string
	ControlAccountName string
	IsActive           bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}

type ARAPTransactionType string

const (
	ARAPTransactionTypeReceivable ARAPTransactionType = "receivable"
	ARAPTransactionTypePayable    ARAPTransactionType = "payable"
)

func (t ARAPTransactionType) IsValid() bool {
	return t == ARAPTransactionTypeReceivable || t == ARAPTransactionTypePayable
}

type ARAPTransactionStatus string

const (
	ARAPStatusOpen          ARAPTransactionStatus = "open"
	ARAPStatusPartiallyPaid ARAPTransactionStatus = "partially_paid"
	ARAPStatusPaid          ARAPTransactionStatus = "paid"
	ARAPStatusVoid          ARAPTransactionStatus = "void"
)

type ARAPTransaction struct {
	ID                string
	PartnerID         string
	PartnerCode       string
	PartnerName       string
	TransactionType   ARAPTransactionType
	TransactionDate   time.Time
	DueDate           time.Time
	ReferenceNumber   string
	Amount            string
	ContraAccountID   string
	ContraAccountCode string
	ContraAccountName string
	Description       string
	Status            ARAPTransactionStatus
	JournalEntryID    string
	CreatedBy         *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	PaidAmount        string
	OutstandingAmount string
}

type ARAPPayment struct {
	ID                   string
	PartnerID            string
	PaymentDate          time.Time
	Amount               string
	CashBankAccountID    string
	CashBankAccountLabel string
	JournalEntryID       string
	Notes                string
	CreatedBy            *string
	CreatedAt            time.Time
}
