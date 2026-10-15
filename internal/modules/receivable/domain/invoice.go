package domain

import (
	"math/big"
	"time"

	"zyad.cloud/internal/shared/pricing"
)

type InvoiceStatus string
type SourceType string

const (
	InvoiceDraft   InvoiceStatus = "draft"
	InvoiceIssued  InvoiceStatus = "issued"
	InvoicePaid    InvoiceStatus = "paid"
	InvoiceOverdue InvoiceStatus = "overdue"
	InvoiceVoid    InvoiceStatus = "void"

	SourceSalesOrder SourceType = "sales_order"
	SourceContract   SourceType = "contract"
	SourceManual     SourceType = "manual"
)

type InvoiceItem struct {
	ID, Description, Quantity, Unit, UnitPrice, DiscountPercent, TaxPercent, TaxAmount, LineTotal string
	ProductID, SKU, ContractItemID, SourceLineID                                                  string
	Pricing                                                                                       pricing.Attributes
	PeriodStart                                                                                   *time.Time
	PeriodEnd                                                                                     *time.Time
	Position                                                                                      int
}

type Invoice struct {
	ID, AccountID, InvoiceNumber                                        string
	Account                                                             Account // join read-only
	Status                                                              InvoiceStatus
	SourceType                                                          SourceType
	SourceID, ContractID, IdempotencyKey                                string
	PeriodStart, PeriodEnd, IssueDate, DueDate                          *time.Time // tanggal kalender (UTC midnight)
	Currency, Subtotal, DiscountTotal, TaxTotal, GrandTotal, AmountPaid string
	Channels                                                            []string
	PICUserID                                                           string
	PDFAssetID                                                          *string
	Notes                                                               string
	PaidAt, VoidedAt                                                    *time.Time
	VoidReason                                                          string
	Items                                                               []InvoiceItem
	CreatedBy                                                           string
	CreatedAt, UpdatedAt                                                time.Time
}

// Balance = GrandTotal - AmountPaid sebagai string desimal 2 digit.
func (i Invoice) Balance() string {
	grand, ok1 := new(big.Rat).SetString(i.GrandTotal)
	paid, ok2 := new(big.Rat).SetString(i.AmountPaid)
	if !ok1 || !ok2 {
		return "0.00"
	}
	return new(big.Rat).Sub(grand, paid).FloatString(2)
}
