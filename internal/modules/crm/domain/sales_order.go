package domain

import (
	"errors"
	"time"

	"zyad.cloud/internal/shared/pricing"
)

type SalesOrderStatus string
type BillingStatus string
type DeliveryStatus string

const (
	SalesOrderDraft     SalesOrderStatus = "draft"
	SalesOrderConfirmed SalesOrderStatus = "confirmed"
	SalesOrderCompleted SalesOrderStatus = "completed"
	SalesOrderCancelled SalesOrderStatus = "cancelled"

	BillingNone    BillingStatus = "none"
	BillingPending BillingStatus = "pending"
	BillingDone    BillingStatus = "done"
	BillingFailed  BillingStatus = "failed"

	DeliveryNotApplicable DeliveryStatus = "not_applicable"
	DeliveryPending       DeliveryStatus = "pending"
	DeliveryDelivered     DeliveryStatus = "delivered"
)

var (
	ErrSalesOrderNotDraft = errors.New("sales order is not draft")
	ErrDeliveryNotPending = errors.New("item is not awaiting delivery confirmation")
)

type SalesOrderItem struct {
	ID, Description, Quantity, Unit, UnitPrice, DiscountPercent, TaxPercent, TaxAmount, LineTotal, ProductID, SKU string
	Pricing                                                                                                       pricing.Attributes
	DeliveryStatus                                                                                                DeliveryStatus
	DeliveredAt                                                                                                   *time.Time
	DeliveredBy, DeliveryNote, InvoiceID                                                                          string
	Position                                                                                                      int
}

type SalesOrder struct {
	ID, SONumber, QuotationID, QuotationNumber                         string
	DealID, ContactID, CompanyID                                       *string
	Status                                                             SalesOrderStatus
	BillingStatus                                                      BillingStatus
	BillingError                                                       string
	StartDate                                                          *time.Time
	BillToName, BillToCompany, BillToEmail, BillToPhone, BillToAddress string
	Channels                                                           []string
	PICUserID, Currency                                                string
	Subtotal, TaxTotal, GrandTotal, FirstInvoiceTotal                  string
	RecurringTotals                                                    map[pricing.Frequency]string
	ReceivableAccountID, InitialInvoiceID, ContractID                  string
	Items                                                              []SalesOrderItem
	ConfirmedAt, CancelledAt                                           *time.Time
	ConfirmedBy, CreatedBy                                             string
	CreatedAt, UpdatedAt                                               time.Time
}
