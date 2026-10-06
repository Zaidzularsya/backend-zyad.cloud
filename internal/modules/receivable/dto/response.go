package dto

import (
	"time"

	"zyad.cloud/internal/modules/receivable/domain"
)

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

type ContractItemResponse struct {
	ID              string `json:"id"`
	Description     string `json:"description"`
	Quantity        string `json:"quantity"`
	Unit            string `json:"unit"`
	UnitPrice       string `json:"unit_price"`
	DiscountPercent string `json:"discount_percent"`
	TaxPercent      string `json:"tax_percent"`
	Frequency       string `json:"billing_frequency"`
	PaymentTiming   string `json:"payment_timing"`
	PeriodIndex     int    `json:"period_index"`
	NextPeriodStart string `json:"next_period_start"`
	NextPeriodEnd   string `json:"next_period_end"`
}

type ContractResponse struct {
	ID             string                 `json:"id"`
	ContractNumber string                 `json:"contract_number"`
	Status         string                 `json:"status"`
	Account        AccountResponse        `json:"account"`
	SourceType     string                 `json:"source_type"`
	SourceID       string                 `json:"source_id"`
	Currency       string                 `json:"currency"`
	StartDate      string                 `json:"start_date"`
	EndDate        *string                `json:"end_date"`
	EndReason      string                 `json:"end_reason"`
	EndedAt        *time.Time             `json:"ended_at"`
	Channels       []string               `json:"channels"`
	PICUserID      string                 `json:"pic_user_id"`
	Notes          string                 `json:"notes"`
	Items          []ContractItemResponse `json:"items"`
	CreatedAt      time.Time              `json:"created_at"`
}

func ContractFromDomain(c domain.Contract) ContractResponse {
	items := make([]ContractItemResponse, 0, len(c.Items))
	for _, it := range c.Items {
		items = append(items, ContractItemResponse{
			ID: it.ID, Description: it.Description, Quantity: it.Quantity, Unit: it.Unit, UnitPrice: it.UnitPrice,
			DiscountPercent: it.DiscountPercent, TaxPercent: it.TaxPercent, Frequency: string(it.Frequency),
			PaymentTiming: string(it.PaymentTiming), PeriodIndex: it.PeriodIndex,
			NextPeriodStart: it.NextPeriodStart.Format(dateLayout), NextPeriodEnd: it.NextPeriodEnd.Format(dateLayout),
		})
	}
	channels := c.Channels
	if channels == nil {
		channels = []string{}
	}
	return ContractResponse{
		ID: c.ID, ContractNumber: c.ContractNumber, Status: string(c.Status), Account: AccountFromDomain(c.Account),
		SourceType: string(c.SourceType), SourceID: c.SourceID, Currency: c.Currency, StartDate: c.StartDate.Format(dateLayout),
		EndDate: DateString(c.EndDate), EndReason: c.EndReason, EndedAt: c.EndedAt, Channels: channels, PICUserID: c.PICUserID,
		Notes: c.Notes, Items: items, CreatedAt: c.CreatedAt,
	}
}

type ContractListQuery struct {
	Status    string `form:"status"`
	AccountID string `form:"account_id"`
	Search    string `form:"search"`
	Page      int    `form:"page"`
	PerPage   int    `form:"per_page"`
}

// ContractPatchRequest: end_date kosong/null menghapus tanggal akhir.
type ContractPatchRequest struct {
	EndDate *string `json:"end_date"`
}

type ContractEndRequest struct {
	EndDate string `json:"end_date"`
	Reason  string `json:"reason"`
}
