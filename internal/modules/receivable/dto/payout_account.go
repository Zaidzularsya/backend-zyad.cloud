package dto

import (
	"time"

	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/modules/receivable/service"
)

// PayoutAccountResponse: nomor rekening dikirim utuh (owner dan super admin perlu mencocokkan).
type PayoutAccountResponse struct {
	ID                string     `json:"id"`
	OrganizationID    string     `json:"organization_id"`
	BankCode          string     `json:"bank_code"`
	BankName          string     `json:"bank_name"`
	AccountNumber     string     `json:"account_number"`
	AccountHolderName string     `json:"account_holder_name"`
	Status            string     `json:"status"`
	SubmittedAt       time.Time  `json:"submitted_at"`
	ReviewedAt        *time.Time `json:"reviewed_at"`
	ReviewNote        string     `json:"review_note"`
}

// PendingPayoutAccountResponse: antrean verifikasi lintas organisasi.
type PendingPayoutAccountResponse struct {
	PayoutAccountResponse
	OrganizationName string `json:"organization_name"`
}

type PayoutOverviewResponse struct {
	Verified *PayoutAccountResponse  `json:"verified"`
	Pending  *PayoutAccountResponse  `json:"pending"`
	History  []PayoutAccountResponse `json:"history"`
}

type BankResponse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// PayoutAccountRequest dipakai submit owner (tanpa note) dan assign super admin (note wajib).
// organization_id sengaja tidak ada: target org selalu dari konteks tenant atau path.
type PayoutAccountRequest struct {
	BankCode          string `json:"bank_code"`
	AccountNumber     string `json:"account_number"`
	AccountHolderName string `json:"account_holder_name"`
	Note              string `json:"note"`
}

type PayoutReviewRequest struct {
	Note string `json:"note"`
}

type PayoutPendingQuery struct {
	Status  string `form:"status"`
	Page    int    `form:"page"`
	PerPage int    `form:"per_page"`
}

// SettlementPayoutRef: rekening pada entri ledger; nomor selalu dimask.
type SettlementPayoutRef struct {
	BankName            string `json:"bank_name"`
	AccountNumberMasked string `json:"account_number_masked"`
}

func PayoutAccountFromRepo(a repository.PayoutAccount) PayoutAccountResponse {
	return PayoutAccountResponse{ID: a.ID, OrganizationID: a.OrganizationID, BankCode: a.BankCode, BankName: a.BankName,
		AccountNumber: a.AccountNumber, AccountHolderName: a.AccountHolderName, Status: a.Status,
		SubmittedAt: a.SubmittedAt, ReviewedAt: a.ReviewedAt, ReviewNote: a.ReviewNote}
}

func payoutAccountPtr(a *repository.PayoutAccount) *PayoutAccountResponse {
	if a == nil {
		return nil
	}
	r := PayoutAccountFromRepo(*a)
	return &r
}

func PayoutOverviewFromService(o service.PayoutOverview) PayoutOverviewResponse {
	hist := make([]PayoutAccountResponse, 0, len(o.History))
	for _, h := range o.History {
		hist = append(hist, PayoutAccountFromRepo(h))
	}
	return PayoutOverviewResponse{Verified: payoutAccountPtr(o.Verified), Pending: payoutAccountPtr(o.Pending), History: hist}
}

func BanksResponse() []BankResponse {
	list := service.Banks()
	out := make([]BankResponse, 0, len(list))
	for _, b := range list {
		out = append(out, BankResponse{Code: b.Code, Name: b.Name})
	}
	return out
}
