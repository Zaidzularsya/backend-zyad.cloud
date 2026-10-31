package dto

import (
	"time"

	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/service"
)

const dateLayout = "2006-01-02"

type SetPolicyRequest struct {
	Exempt *bool  `json:"exempt" binding:"required"`
	Reason string `json:"reason" binding:"max=500"`
}

type SetTriggerRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

type PolicyResponse struct {
	AccountID string     `json:"account_id"`
	Exempt    bool       `json:"exempt"`
	Reason    string     `json:"reason"`
	SetAt     *time.Time `json:"set_at"`
}

func PolicyFromDomain(p domain.CustomerPolicy) PolicyResponse {
	out := PolicyResponse{AccountID: p.AccountID, Exempt: p.Exempt, Reason: p.Reason}
	if !p.SetAt.IsZero() {
		at := p.SetAt
		out.SetAt = &at
	}
	return out
}

type ProductResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SKU       string `json:"sku"`
	Recurring bool   `json:"recurring"`
	Active    bool   `json:"active"`
}

func ProductFromService(p service.ProductInfo) ProductResponse {
	return ProductResponse{ID: p.ID, Name: p.Name, SKU: p.SKU, Recurring: p.Recurring, Active: p.Active}
}

func ProductListFromService(products []service.ProductInfo) []ProductResponse {
	out := make([]ProductResponse, 0, len(products))
	for _, p := range products {
		out = append(out, ProductFromService(p))
	}
	return out
}

type TriggerResponse struct {
	ProductID     string `json:"product_id"`
	Enabled       bool   `json:"enabled"`
	EffectiveFrom string `json:"effective_from"`
}

func TriggerFromDomain(t domain.ProductTrigger) TriggerResponse {
	return TriggerResponse{ProductID: t.ProductID, Enabled: t.Enabled, EffectiveFrom: t.EffectiveFrom.Format(dateLayout)}
}

type TriggerPreviewResponse struct {
	Product         ProductResponse `json:"product"`
	ActiveContracts int             `json:"active_contracts"`
	Accounts        int             `json:"accounts"`
	OverdueInvoices int             `json:"overdue_invoices"`
	EffectiveFrom   string          `json:"effective_from"`
	AlreadyEnabled  bool            `json:"already_enabled"`
}

func TriggerPreviewFromService(p service.TriggerPreview) TriggerPreviewResponse {
	return TriggerPreviewResponse{
		Product: ProductFromService(p.Product), ActiveContracts: p.Usage.ActiveContracts,
		Accounts: p.Usage.Accounts, OverdueInvoices: p.Usage.OverdueInvoices,
		EffectiveFrom: p.EffectiveFrom.Format(dateLayout), AlreadyEnabled: p.AlreadyEnabled,
	}
}
