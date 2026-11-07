package dto

import (
	"zyad.cloud/internal/modules/mikrotik/service"
)

// PreviewQuery filters the isolation candidate preview and its CSV export.
type PreviewQuery struct {
	Status  string `form:"status" binding:"omitempty,oneof=exempt no_secret already_isolated needs_profile secret_missing router_not_ready will_isolate"`
	Q       string `form:"q" binding:"max=100"`
	Page    int    `form:"page"`
	PerPage int    `form:"per_page"`
}

func (q PreviewQuery) Filter() service.CandidateFilter {
	return service.CandidateFilter{Status: q.Status, Q: q.Q}
}

type PreviewSummaryResponse struct {
	Target          int `json:"target"`
	Exempt          int `json:"exempt"`
	Blocked         int `json:"blocked"`
	AlreadyIsolated int `json:"already_isolated"`
	PairedAccounts  int `json:"paired_accounts"`
	Safe            int `json:"safe"`
	GraceDays       int `json:"grace_days"`
	// Cutoff (YYYY-MM-DD): invoices due before this date are candidates.
	Cutoff string `json:"cutoff"`
}

func PreviewSummaryFromService(s service.CandidateSummary) PreviewSummaryResponse {
	return PreviewSummaryResponse{
		Target: s.Target, Exempt: s.Exempt, Blocked: s.Blocked, AlreadyIsolated: s.AlreadyIsolated,
		PairedAccounts: s.PairedAccounts, Safe: s.Safe, GraceDays: s.GraceDays, Cutoff: s.Cutoff.Format(dateLayout),
	}
}

type PreviewItemResponse struct {
	AccountID           string  `json:"account_id"`
	AccountName         string  `json:"account_name"`
	Phone               string  `json:"phone"`
	Status              string  `json:"status"`
	OverdueDays         int     `json:"overdue_days"`
	InvoiceCount        int     `json:"invoice_count"`
	OldestInvoiceNumber string  `json:"oldest_invoice_number"`
	Balance             string  `json:"balance"`
	RouterName          string  `json:"router_name"`
	Products            string  `json:"products"`
	Pilot               bool    `json:"pilot"`
	ExemptedUntil       *string `json:"exempted_until"`
}

func PreviewListFromService(rows []service.CandidateRow) []PreviewItemResponse {
	out := make([]PreviewItemResponse, 0, len(rows))
	for _, r := range rows {
		it := PreviewItemResponse{
			AccountID: r.AccountID, AccountName: r.AccountName, Phone: r.Phone, Status: r.Status,
			OverdueDays: r.OverdueDays, InvoiceCount: r.InvoiceCount, OldestInvoiceNumber: r.OldestInvoiceNumber,
			Balance: r.Balance, RouterName: r.RouterName, Products: r.Products, Pilot: r.Pilot,
		}
		if r.ExemptUntil != nil {
			d := r.ExemptUntil.Format(dateLayout)
			it.ExemptedUntil = &d
		}
		out = append(out, it)
	}
	return out
}
