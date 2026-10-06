package dto

import (
	"time"

	"zyad.cloud/internal/modules/crm/domain"
)

type PublicApproveRequest struct {
	ResponderName string `json:"responder_name"`
	Agree         bool   `json:"agree"`
}

type PublicRevisionRequest struct {
	ResponderName string   `json:"responder_name"`
	Categories    []string `json:"categories"`
	Note          string   `json:"note"`
}

type PublicQuotationItem struct {
	Description      string   `json:"description"`
	Quantity         string   `json:"quantity"`
	Unit             string   `json:"unit,omitempty"`
	UnitPrice        string   `json:"unit_price"`
	LineTotal        string   `json:"line_total"`
	ChargeType       string   `json:"charge_type,omitempty"`
	BillingFrequency *string  `json:"billing_frequency,omitempty"`
	PaymentTiming    string   `json:"payment_timing,omitempty"`
	Features         []string `json:"features,omitempty"` // label terlihat dari snapshot baris
}

type PublicQuotationLastResponse struct {
	Action        string    `json:"action"`
	ResponderName string    `json:"responder_name"`
	Categories    []string  `json:"categories,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// PublicQuotationResponse sengaja tidak memuat ID internal, catatan internal,
// atau data kontak: halaman publik hanya butuh isi penawaran.
type PublicQuotationResponse struct {
	TenantName        string                       `json:"tenant_name"`
	QuotationNumber   string                       `json:"quotation_number"`
	Status            string                       `json:"status"`
	State             string                       `json:"state"`
	ValidUntil        *time.Time                   `json:"valid_until,omitempty"`
	Currency          string                       `json:"currency"`
	GrandTotal        string                       `json:"grand_total"`
	OneTimeTotal      string                       `json:"one_time_total"`
	FirstInvoiceTotal string                       `json:"first_invoice_total"`
	RecurringTotals   map[string]string            `json:"recurring_totals"`
	Items             []PublicQuotationItem        `json:"items"`
	LastResponse      *PublicQuotationLastResponse `json:"last_response"`
}

type QuotationLinkResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type QuotationResponseItem struct {
	ID            string    `json:"id"`
	Action        string    `json:"action"`
	Categories    []string  `json:"categories"`
	Note          string    `json:"note,omitempty"`
	ResponderName string    `json:"responder_name"`
	CreatedAt     time.Time `json:"created_at"`
}

func QuotationResponsesFromDomain(list []domain.QuotationResponse) []QuotationResponseItem {
	out := make([]QuotationResponseItem, 0, len(list))
	for _, r := range list {
		item := QuotationResponseItem{ID: r.ID, Action: string(r.Action), Categories: []string{}, Note: r.Note,
			ResponderName: r.ResponderName, CreatedAt: r.CreatedAt}
		for _, c := range r.Categories {
			item.Categories = append(item.Categories, string(c))
		}
		out = append(out, item)
	}
	return out
}
