package dto

import (
	"time"

	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/modules/receivable/service"
)

type SettlementBalanceResponse struct {
	PaidIn   string `json:"paid_in"`
	Settled  string `json:"settled"`
	Adjusted string `json:"adjusted"`
	// Balance = paid_in - settled + adjusted (sisa yang masih menjadi hak organisasi).
	Balance string `json:"balance"`
}

type SettlementOrgBalanceResponse struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	SettlementBalanceResponse
}

type SettlementEntryResponse struct {
	ID                string    `json:"id"`
	OrganizationID    string    `json:"organization_id"`
	Kind              string    `json:"kind"`
	Amount            string    `json:"amount"`
	InvoiceID         string    `json:"invoice_id,omitempty"`
	ProviderReference string    `json:"provider_reference,omitempty"`
	Note              string    `json:"note"`
	OccurredAt        time.Time `json:"occurred_at"`
	CreatedAt         time.Time `json:"created_at"`
}

// SettlementEntryRequest: occurred_at opsional (RFC 3339); kosong = sekarang.
type SettlementEntryRequest struct {
	Kind       string     `json:"kind"`
	Amount     string     `json:"amount"`
	Note       string     `json:"note"`
	OccurredAt *time.Time `json:"occurred_at"`
}

type SettlementListQuery struct {
	Search  string `form:"search"`
	Page    int    `form:"page"`
	PerPage int    `form:"per_page"`
}

func SettlementBalanceFromService(b service.SettlementBalance) SettlementBalanceResponse {
	return SettlementBalanceResponse{PaidIn: b.PaidIn, Settled: b.Settled, Adjusted: b.Adjusted, Balance: b.Remaining}
}

func SettlementOrgBalanceFromService(b service.OrgBalance) SettlementOrgBalanceResponse {
	return SettlementOrgBalanceResponse{OrganizationID: b.OrganizationID, OrganizationName: b.OrganizationName, SettlementBalanceResponse: SettlementBalanceFromService(b.SettlementBalance)}
}

func SettlementEntryFromRepo(e repository.SettlementEntry) SettlementEntryResponse {
	return SettlementEntryResponse{ID: e.ID, OrganizationID: e.OrganizationID, Kind: e.Kind, Amount: e.Amount, InvoiceID: e.InvoiceID,
		ProviderReference: e.ProviderReference, Note: e.Note, OccurredAt: e.OccurredAt, CreatedAt: e.CreatedAt}
}
