package model

import "time"

type CashBankAccountType string

const (
	CashBankAccountTypeCash CashBankAccountType = "cash"
	CashBankAccountTypeBank CashBankAccountType = "bank"
)

func (t CashBankAccountType) IsValid() bool {
	return t == CashBankAccountTypeCash || t == CashBankAccountTypeBank
}

type CashBankAccount struct {
	ID                string
	AccountID         string
	AccountCode       string
	AccountName       string
	Type              CashBankAccountType
	BankName          string
	AccountNumber     string
	AccountHolderName string
	Currency          string
	IsActive          bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type CashTransactionType string

const (
	CashTransactionTypeCashIn   CashTransactionType = "cash_in"
	CashTransactionTypeCashOut  CashTransactionType = "cash_out"
	CashTransactionTypeTransfer CashTransactionType = "transfer"
)

func (t CashTransactionType) IsValid() bool {
	switch t {
	case CashTransactionTypeCashIn, CashTransactionTypeCashOut, CashTransactionTypeTransfer:
		return true
	default:
		return false
	}
}

type CashTransaction struct {
	ID                       string
	CashBankAccountID        string
	CashBankAccountCode      string
	CashBankAccountLabel     string
	TransactionDate          time.Time
	TransactionType          CashTransactionType
	Amount                   string
	CounterCashBankAccountID *string
	CounterCashBankLabel     string
	ContraAccountID          *string
	ContraAccountCode        string
	ContraAccountName        string
	Reference                string
	Description              string
	JournalEntryID           string
	ReconciledAt             *time.Time
	ReconciledBy             *string
	CreatedBy                *string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type BankReconciliationStatus string

const (
	BankReconciliationStatusDraft     BankReconciliationStatus = "draft"
	BankReconciliationStatusCompleted BankReconciliationStatus = "completed"
)

type BankReconciliation struct {
	ID                     string
	CashBankAccountID      string
	StatementDate          time.Time
	StatementEndingBalance string
	BookEndingBalance      string
	Status                 BankReconciliationStatus
	Notes                  string
	CompletedAt            *time.Time
	CompletedBy            *string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}
