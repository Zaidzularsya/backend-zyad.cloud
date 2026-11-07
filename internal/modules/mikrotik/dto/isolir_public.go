package dto

import "zyad.cloud/internal/modules/mikrotik/service"

// IsolirLookupRequest is the public lookup body. A malformed body is treated
// like an empty query by the handler (uniform answer, no probing oracle).
type IsolirLookupRequest struct {
	Query string `json:"query"`
}

type IsolirTenantResponse struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email"`
}

// IsolirInfoResponse omits tenant when the page is disabled.
type IsolirInfoResponse struct {
	Enabled bool                  `json:"enabled"`
	Tenant  *IsolirTenantResponse `json:"tenant,omitempty"`
}

func IsolirInfoFromService(i service.PublicInfo) IsolirInfoResponse {
	out := IsolirInfoResponse{Enabled: i.Enabled}
	if i.Enabled && i.Tenant != nil {
		out.Tenant = &IsolirTenantResponse{Name: i.Tenant.Name, Phone: i.Tenant.Phone, Email: i.Tenant.Email}
	}
	return out
}

type IsolirInvoiceResponse struct {
	Number  string `json:"number"`
	DueDate string `json:"due_date"`
	Balance string `json:"balance"`
	PayURL  string `json:"pay_url"`
}

// IsolirLookupResponse never carries phone, email or infrastructure names.
type IsolirLookupResponse struct {
	Found      bool                    `json:"found"`
	MaskedName string                  `json:"masked_name"`
	Isolated   bool                    `json:"isolated"`
	Invoices   []IsolirInvoiceResponse `json:"invoices"`
}

func IsolirLookupFromService(r service.PublicLookupResult) IsolirLookupResponse {
	out := IsolirLookupResponse{
		Found: r.Found, MaskedName: r.MaskedName, Isolated: r.Isolated,
		Invoices: make([]IsolirInvoiceResponse, 0, len(r.Invoices)),
	}
	for _, inv := range r.Invoices {
		out.Invoices = append(out.Invoices, IsolirInvoiceResponse{
			Number: inv.Number, DueDate: inv.DueDate.Format(dateLayout), Balance: inv.Balance, PayURL: inv.PayURL,
		})
	}
	return out
}
