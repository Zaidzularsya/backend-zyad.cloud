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

// PaginationMeta dikirim sebagai "meta" amplop respons (bukan di dalam "data").
type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

func BuildMeta(page, perPage int, total int64) PaginationMeta {
	totalPages := 0
	if perPage > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}
	return PaginationMeta{Page: page, PerPage: perPage, Total: total, TotalPages: totalPages}
}

type AccountResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	CompanyName string    `json:"company_name"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	Address     string    `json:"address"`
	ContactID   *string   `json:"contact_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type InvoiceItemResponse struct {
	ID               string  `json:"id"`
	Description      string  `json:"description"`
	Quantity         string  `json:"quantity"`
	Unit             string  `json:"unit"`
	UnitPrice        string  `json:"unit_price"`
	DiscountPercent  *string `json:"discount_percent"`
	TaxPercent       string  `json:"tax_percent"`
	TaxAmount        string  `json:"tax_amount"`
	LineTotal        string  `json:"line_total"`
	ProductID        *string `json:"product_id"`
	SKU              string  `json:"sku"`
	ChargeType       string  `json:"charge_type"`
	BillingFrequency *string `json:"billing_frequency"`
	PaymentTiming    string  `json:"payment_timing"`
	PeriodStart      *string `json:"period_start"`
	PeriodEnd        *string `json:"period_end"`
	Position         int     `json:"position"`
}

type InvoiceSendResponse struct {
	ID         string    `json:"id"`
	InvoiceID  string    `json:"invoice_id"`
	Channel    string    `json:"channel"`
	Recipient  string    `json:"recipient"`
	Status     string    `json:"status"`
	Error      string    `json:"error"`
	Trigger    string    `json:"trigger"`
	SentBy     *string   `json:"sent_by"`
	SentByName string    `json:"sent_by_name"`
	SentAt     time.Time `json:"sent_at"`
}

type InvoiceResponse struct {
	ID            string                `json:"id"`
	InvoiceNumber *string               `json:"invoice_number"`
	Status        string                `json:"status"`
	Account       AccountResponse       `json:"account"`
	SourceType    string                `json:"source_type"`
	SourceID      *string               `json:"source_id"`
	ContractID    *string               `json:"contract_id"`
	PeriodStart   *string               `json:"period_start"`
	PeriodEnd     *string               `json:"period_end"`
	IssueDate     *string               `json:"issue_date"`
	DueDate       *string               `json:"due_date"`
	Currency      string                `json:"currency"`
	Subtotal      string                `json:"subtotal"`
	DiscountTotal string                `json:"discount_total"`
	TaxTotal      string                `json:"tax_total"`
	GrandTotal    string                `json:"grand_total"`
	AmountPaid    string                `json:"amount_paid"`
	Balance       string                `json:"balance"`
	Channels      []string              `json:"channels"`
	PICUserID     *string               `json:"pic_user_id"`
	Notes         string                `json:"notes"`
	VoidReason    string                `json:"void_reason"`
	PaidAt        *time.Time            `json:"paid_at"`
	VoidedAt      *time.Time            `json:"voided_at"`
	Items         []InvoiceItemResponse `json:"items"`
	LastSend      *InvoiceSendResponse  `json:"last_send"`
	CreatedAt     time.Time             `json:"created_at"`
}

type InvoiceLinkResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type PaymentResponse struct {
	ID            string    `json:"id"`
	InvoiceID     string    `json:"invoice_id"`
	InvoiceNumber string    `json:"invoice_number,omitempty"`
	AccountName   string    `json:"account_name,omitempty"`
	Amount        string    `json:"amount"`
	Method        string    `json:"method"`
	Reference     string    `json:"reference"`
	Note          string    `json:"note"`
	RecordedBy    *string   `json:"recorded_by"`
	PaidAt        time.Time `json:"paid_at"`
	CreatedAt     time.Time `json:"created_at"`
}

type RecordPaymentResponse struct {
	Invoice InvoiceResponse `json:"invoice"`
	Payment PaymentResponse `json:"payment"`
}

type SettingsResponse struct {
	InvoiceLeadDays     int      `json:"invoice_lead_days"`
	PaymentTermsDays    int      `json:"payment_terms_days"`
	DefaultChannels     []string `json:"default_channels"`
	DefaultSenderUserID *string  `json:"default_sender_user_id"`
}

type SenderResponse struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
}
