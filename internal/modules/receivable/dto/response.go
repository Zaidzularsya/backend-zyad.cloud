package dto

import "time"

const dateLayout = "2006-01-02"

// DateString memformat tanggal kalender (UTC midnight) sebagai YYYY-MM-DD; nil → nil.
func DateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dateLayout)
	return &s
}

type PublicInvoiceItem struct {
	Description      string  `json:"description"`
	Quantity         string  `json:"quantity"`
	Unit             string  `json:"unit"`
	UnitPrice        string  `json:"unit_price"`
	LineTotal        string  `json:"line_total"`
	ChargeType       string  `json:"charge_type"`
	BillingFrequency *string `json:"billing_frequency"`
	PaymentTiming    string  `json:"payment_timing"`
	PeriodStart      *string `json:"period_start"`
	PeriodEnd        *string `json:"period_end"`
}

// PublicInvoiceResponse adalah isi halaman publik /i/:token. Tidak memuat id internal.
type PublicInvoiceResponse struct {
	TenantName    string              `json:"tenant_name"`
	InvoiceNumber string              `json:"invoice_number"`
	Status        string              `json:"status"`
	State         string              `json:"state"`
	IssueDate     *string             `json:"issue_date"`
	DueDate       *string             `json:"due_date"`
	PeriodStart   *string             `json:"period_start"`
	PeriodEnd     *string             `json:"period_end"`
	Currency      string              `json:"currency"`
	GrandTotal    string              `json:"grand_total"`
	AmountPaid    string              `json:"amount_paid"`
	Balance       string              `json:"balance"`
	Items         []PublicInvoiceItem `json:"items"`
	CanPay        bool                `json:"can_pay"`
}
