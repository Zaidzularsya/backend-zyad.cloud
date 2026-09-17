package model

import "time"

type JournalStatus string

const (
	JournalStatusDraft    JournalStatus = "draft"
	JournalStatusPosted   JournalStatus = "posted"
	JournalStatusReversed JournalStatus = "reversed"
)

func (s JournalStatus) IsValid() bool {
	switch s {
	case JournalStatusDraft, JournalStatusPosted, JournalStatusReversed:
		return true
	default:
		return false
	}
}

type JournalSourceType string

const (
	JournalSourceManual         JournalSourceType = "manual"
	JournalSourceOpeningBalance JournalSourceType = "opening_balance"
	JournalSourceDepreciation   JournalSourceType = "depreciation"
	JournalSourceAR             JournalSourceType = "ar"
	JournalSourceAP             JournalSourceType = "ap"
	JournalSourceTax            JournalSourceType = "tax"
)

type SubledgerType string

const (
	SubledgerNone     SubledgerType = "none"
	SubledgerCustomer SubledgerType = "customer"
	SubledgerVendor   SubledgerType = "vendor"
	SubledgerAsset    SubledgerType = "asset"
	SubledgerTaxCode  SubledgerType = "tax_code"
)

type JournalEntry struct {
	ID                string
	EntryNumber       string
	EntryDate         time.Time
	FiscalPeriodID    string
	SourceType        JournalSourceType
	SourceID          *string
	Reference         string
	Description       string
	Status            JournalStatus
	PostedAt          *time.Time
	PostedBy          *string
	ReversedByEntryID *string
	CreatedBy         *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Lines             []JournalLine
}

type JournalLine struct {
	ID             string
	JournalEntryID string
	LineNumber     int
	AccountID      string
	AccountCode    string
	AccountName    string
	Debit          string
	Credit         string
	Description    string
	SubledgerType  SubledgerType
	SubledgerID    *string
	CreatedAt      time.Time
}
