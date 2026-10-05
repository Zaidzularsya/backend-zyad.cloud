package dto

type AccountRequest struct {
	Name        string `json:"name"`
	CompanyName string `json:"company_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Address     string `json:"address"`
}

type AccountListQuery struct {
	Search  string `form:"search"`
	Page    int    `form:"page"`
	PerPage int    `form:"per_page"`
}

type InvoiceItemRequest struct {
	ProductID        string `json:"product_id"`
	SKU              string `json:"sku"`
	Description      string `json:"description"`
	Quantity         string `json:"quantity"`
	Unit             string `json:"unit"`
	UnitPrice        string `json:"unit_price"`
	DiscountPercent  string `json:"discount_percent"`
	TaxPercent       string `json:"tax_percent"`
	ChargeType       string `json:"charge_type"`
	BillingFrequency string `json:"billing_frequency"`
	PaymentTiming    string `json:"payment_timing"`
	PeriodStart      string `json:"period_start"` // YYYY-MM-DD
	PeriodEnd        string `json:"period_end"`
}

// InvoiceRequest membuat/mengubah draft invoice manual. Sumber (sales order/kontrak) tidak
// dibuka lewat HTTP; itu diisi modul lain lewat service.
type InvoiceRequest struct {
	AccountID   string               `json:"account_id"`
	Channels    []string             `json:"channels"`
	PICUserID   string               `json:"pic_user_id"`
	Notes       string               `json:"notes"`
	Currency    string               `json:"currency"`
	PeriodStart string               `json:"period_start"`
	PeriodEnd   string               `json:"period_end"`
	Items       []InvoiceItemRequest `json:"items"`
}

type InvoiceListQuery struct {
	Status     string `form:"status"`
	AccountID  string `form:"account_id"`
	SourceType string `form:"source_type"`
	SourceID   string `form:"source_id"`
	ContractID string `form:"contract_id"`
	SendFailed bool   `form:"send_failed"`
	Search     string `form:"search"`
	Page       int    `form:"page"`
	PerPage    int    `form:"per_page"`
}

type VoidInvoiceRequest struct {
	Reason string `json:"reason"`
}

type SendInvoiceRequest struct {
	Channel         string `json:"channel"`
	Recipient       string `json:"recipient"`
	Message         string `json:"message"`
	ClientRequestID string `json:"client_request_id"`
}

type RecordPaymentRequest struct {
	Amount    string `json:"amount"`
	PaidAt    string `json:"paid_at"` // RFC3339 atau YYYY-MM-DD; kosong = sekarang
	Reference string `json:"reference"`
	Note      string `json:"note"`
}

type PaymentListQuery struct {
	Page    int `form:"page"`
	PerPage int `form:"per_page"`
}

type SettingsRequest struct {
	InvoiceLeadDays     int      `json:"invoice_lead_days"`
	PaymentTermsDays    int      `json:"payment_terms_days"`
	DefaultChannels     []string `json:"default_channels"`
	DefaultSenderUserID string   `json:"default_sender_user_id"`
}
