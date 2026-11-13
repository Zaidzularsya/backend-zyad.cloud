package dto

import (
	"time"

	"zyad.cloud/internal/modules/receivable/domain"
)

type ArrearsRowRequest struct {
	PeriodStart string `json:"period_start"`
	Amount      string `json:"amount"`
	DueDate     string `json:"due_date"`
	IsolirFlag  bool   `json:"isolir_flag"`
}

type ArrearsRequest struct {
	AccountID  string              `json:"account_id"`
	ContractID string              `json:"contract_id"`
	Reason     string              `json:"reason"`
	Rows       []ArrearsRowRequest `json:"rows"`
}

type IsolirFlagRequest struct {
	Value *bool `json:"value"`
}

type ArrearsPeriodResponse struct {
	Index         int    `json:"index"`
	Start         string `json:"start"`
	End           string `json:"end"`
	DefaultAmount string `json:"default_amount"`
	Conflict      string `json:"conflict"`
}

type ArrearsPeriodsResponse struct {
	Periods           []ArrearsPeriodResponse `json:"periods"`
	HasTriggerProduct bool                    `json:"has_trigger_product"`
}

type ArrearsRecordingResponse struct {
	ID           string     `json:"id"`
	AccountID    string     `json:"account_id"`
	ContractID   string     `json:"contract_id"`
	Reason       string     `json:"reason"`
	RecordedBy   *string    `json:"recorded_by"`
	NotifyStatus string     `json:"notify_status"`
	NotifyError  string     `json:"notify_error"`
	NotifiedAt   *time.Time `json:"notified_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

type ArrearsResultResponse struct {
	Recording    ArrearsRecordingResponse `json:"recording"`
	Invoices     []InvoiceResponse        `json:"invoices"`
	NotifyStatus string                   `json:"notify_status"`
}

type ArrearsConflictResponse struct {
	Row         int    `json:"row"`
	PeriodStart string `json:"period_start"`
	Reason      string `json:"reason"`
}

func ArrearsRecordingFromDomain(r domain.ArrearsRecording) ArrearsRecordingResponse {
	return ArrearsRecordingResponse{
		ID: r.ID, AccountID: r.AccountID, ContractID: r.ContractID, Reason: r.Reason, RecordedBy: strPtr(r.RecordedBy),
		NotifyStatus: r.NotifyStatus, NotifyError: r.NotifyError, NotifiedAt: r.NotifiedAt, CreatedAt: r.CreatedAt,
	}
}
