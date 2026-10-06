package dto

import (
	"time"

	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/service"
)

const soDateLayout = "2006-01-02"

type SalesOrderUpdateRequest struct {
	StartDate     *string  `json:"start_date"`
	BillToName    string   `json:"bill_to_name"`
	BillToCompany string   `json:"bill_to_company"`
	BillToEmail   string   `json:"bill_to_email"`
	BillToPhone   string   `json:"bill_to_phone"`
	BillToAddress string   `json:"bill_to_address"`
	Channels      []string `json:"channels"`
	PICUserID     string   `json:"pic_user_id"`
}

type SalesOrderDeliveryRequest struct {
	ItemIDs     []string `json:"item_ids"`
	DeliveredAt string   `json:"delivered_at"`
	Note        string   `json:"note"`
	BatchKey    string   `json:"batch_key"`
}

type SalesOrderListQuery struct {
	Status        string `form:"status"`
	BillingStatus string `form:"billing_status"`
	DealID        string `form:"deal_id"`
	Search        string `form:"search"`
	Page          int    `form:"page"`
	PerPage       int    `form:"per_page"`
}

type SalesOrderItemResponse struct {
	ID               string  `json:"id"`
	Description      string  `json:"description"`
	Quantity         string  `json:"quantity"`
	Unit             string  `json:"unit"`
	UnitPrice        string  `json:"unit_price"`
	DiscountPercent  string  `json:"discount_percent"`
	TaxPercent       string  `json:"tax_percent"`
	TaxAmount        string  `json:"tax_amount"`
	LineTotal        string  `json:"line_total"`
	ChargeType       string  `json:"charge_type"`
	BillingFrequency *string `json:"billing_frequency"`
	PaymentTiming    string  `json:"payment_timing"`
	DeliveryStatus   string  `json:"delivery_status"`
	DeliveredAt      *string `json:"delivered_at"`
	DeliveryNote     string  `json:"delivery_note"`
	InvoiceID        string  `json:"invoice_id"`
	Position         int     `json:"position"`
}

// DocumentRef: tautan ringkas ke dokumen receivable (invoice awal / kontrak).
type DocumentRef struct {
	ID     string `json:"id"`
	Number string `json:"number"`
	Status string `json:"status"`
}

type SalesOrderResponse struct {
	ID                string                   `json:"id"`
	SONumber          string                   `json:"so_number"`
	QuotationID       string                   `json:"quotation_id"`
	QuotationNumber   string                   `json:"quotation_number"`
	DealID            *string                  `json:"deal_id"`
	ContactID         *string                  `json:"contact_id"`
	CompanyID         *string                  `json:"company_id"`
	Status            string                   `json:"status"`
	BillingStatus     string                   `json:"billing_status"`
	BillingError      string                   `json:"billing_error"`
	StartDate         *string                  `json:"start_date"`
	BillToName        string                   `json:"bill_to_name"`
	BillToCompany     string                   `json:"bill_to_company"`
	BillToEmail       string                   `json:"bill_to_email"`
	BillToPhone       string                   `json:"bill_to_phone"`
	BillToAddress     string                   `json:"bill_to_address"`
	Channels          []string                 `json:"channels"`
	PICUserID         string                   `json:"pic_user_id"`
	Currency          string                   `json:"currency"`
	Subtotal          string                   `json:"subtotal"`
	TaxTotal          string                   `json:"tax_total"`
	GrandTotal        string                   `json:"grand_total"`
	FirstInvoiceTotal string                   `json:"first_invoice_total"`
	RecurringTotals   map[string]string        `json:"recurring_totals"`
	InitialInvoice    *DocumentRef             `json:"initial_invoice"`
	Contract          *DocumentRef             `json:"contract"`
	Items             []SalesOrderItemResponse `json:"items"`
	ConfirmedAt       *time.Time               `json:"confirmed_at"`
	CreatedAt         time.Time                `json:"created_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
}

// SalesOrderFromDomain: invoice/contract diisi pemanggil (handler) karena butuh pembacaan receivable.
func SalesOrderFromDomain(so domain.SalesOrder, invoice, contract *DocumentRef) SalesOrderResponse {
	totals := map[string]string{}
	for f, v := range so.RecurringTotals {
		totals[string(f)] = v
	}
	items := make([]SalesOrderItemResponse, 0, len(so.Items))
	for _, it := range so.Items {
		var freq *string
		if it.Pricing.Frequency != "" {
			f := string(it.Pricing.Frequency)
			freq = &f
		}
		var delivered *string
		if it.DeliveredAt != nil {
			d := it.DeliveredAt.Format(soDateLayout)
			delivered = &d
		}
		items = append(items, SalesOrderItemResponse{
			ID: it.ID, Description: it.Description, Quantity: it.Quantity, Unit: it.Unit, UnitPrice: it.UnitPrice,
			DiscountPercent: it.DiscountPercent, TaxPercent: it.TaxPercent, TaxAmount: it.TaxAmount, LineTotal: it.LineTotal,
			ChargeType: string(it.Pricing.ChargeType), BillingFrequency: freq, PaymentTiming: string(it.Pricing.PaymentTiming),
			DeliveryStatus: string(it.DeliveryStatus), DeliveredAt: delivered, DeliveryNote: it.DeliveryNote,
			InvoiceID: it.InvoiceID, Position: it.Position,
		})
	}
	var start *string
	if so.StartDate != nil {
		d := so.StartDate.Format(soDateLayout)
		start = &d
	}
	channels := so.Channels
	if channels == nil {
		channels = []string{}
	}
	return SalesOrderResponse{
		ID: so.ID, SONumber: so.SONumber, QuotationID: so.QuotationID, QuotationNumber: so.QuotationNumber,
		DealID: so.DealID, ContactID: so.ContactID, CompanyID: so.CompanyID, Status: string(so.Status),
		BillingStatus: string(so.BillingStatus), BillingError: so.BillingError, StartDate: start,
		BillToName: so.BillToName, BillToCompany: so.BillToCompany, BillToEmail: so.BillToEmail, BillToPhone: so.BillToPhone,
		BillToAddress: so.BillToAddress, Channels: channels, PICUserID: so.PICUserID, Currency: so.Currency,
		Subtotal: so.Subtotal, TaxTotal: so.TaxTotal, GrandTotal: so.GrandTotal, FirstInvoiceTotal: so.FirstInvoiceTotal,
		RecurringTotals: totals, InitialInvoice: invoice, Contract: contract, Items: items,
		ConfirmedAt: so.ConfirmedAt, CreatedAt: so.CreatedAt, UpdatedAt: so.UpdatedAt,
	}
}

type WonConditionResponse struct {
	ItemID      string `json:"item_id"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Met         bool   `json:"met"`
	Label       string `json:"label"`
}

type WonChecklistResponse struct {
	SalesOrderID string                 `json:"sales_order_id"`
	SONumber     string                 `json:"so_number"`
	AllMet       bool                   `json:"all_met"`
	Conditions   []WonConditionResponse `json:"conditions"`
}

func WonChecklistsFromService(lists []service.WonChecklist) []WonChecklistResponse {
	out := make([]WonChecklistResponse, 0, len(lists))
	for _, l := range lists {
		conds := make([]WonConditionResponse, 0, len(l.Conditions))
		for _, c := range l.Conditions {
			conds = append(conds, WonConditionResponse{ItemID: c.ItemID, Description: c.Description, Kind: c.Kind, Met: c.Met, Label: c.Label})
		}
		out = append(out, WonChecklistResponse{SalesOrderID: l.SalesOrderID, SONumber: l.SONumber, AllMet: l.AllMet, Conditions: conds})
	}
	return out
}
