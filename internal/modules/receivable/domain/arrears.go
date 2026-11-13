package domain

import "time"

// Notify statuses of an arrears recording's summary message.
const (
	ArrearsNotifyPending = "pending"
	ArrearsNotifySent    = "sent"
	ArrearsNotifyFailed  = "failed"
	ArrearsNotifySkipped = "skipped"
)

// ArrearsRecording is one "catat tunggakan" action: a reason and the
// back-dated invoices it created (receivable_invoices.arrears_recording_id).
type ArrearsRecording struct {
	ID, AccountID, ContractID, Reason, RecordedBy, IdempotencyKey, NotifyStatus, NotifyError string
	NotifiedAt                                                                               *time.Time
	CreatedAt                                                                                time.Time
}
