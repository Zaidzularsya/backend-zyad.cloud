package dto

type CreateAccountRequest struct {
	AccountCode        string  `json:"account_code" binding:"required"`
	AccountName        string  `json:"account_name" binding:"required"`
	AccountCategoryID  string  `json:"account_category_id"`
	ParentAccountID    string  `json:"parent_account_id"`
	IsHeader           bool    `json:"is_header"`
	NormalBalance      string  `json:"normal_balance" binding:"required"`
	OpeningBalance     string  `json:"opening_balance"`
	OpeningBalanceDate *string `json:"opening_balance_date"`
	Description        string  `json:"description"`
}

type UpdateAccountRequest struct {
	AccountName        string  `json:"account_name" binding:"required"`
	AccountCategoryID  string  `json:"account_category_id"`
	ParentAccountID    string  `json:"parent_account_id"`
	IsActive           bool    `json:"is_active"`
	OpeningBalance     string  `json:"opening_balance"`
	OpeningBalanceDate *string `json:"opening_balance_date"`
	Description        string  `json:"description"`
}

type AccountListQuery struct {
	IncludeInactive bool   `form:"include_inactive"`
	CategoryID      string `form:"category_id"`
}

type CreateFiscalYearRequest struct {
	Year int `json:"year" binding:"required"`
}

type JournalEntryListQuery struct {
	Page      int    `form:"page"`
	PerPage   int    `form:"per_page"`
	StartDate string `form:"start_date"`
	EndDate   string `form:"end_date"`
	Status    string `form:"status"`
}

type CreateJournalEntryRequest struct {
	EntryDate   string                     `json:"entry_date" binding:"required"`
	Reference   string                     `json:"reference"`
	Description string                     `json:"description"`
	Lines       []CreateJournalLineRequest `json:"lines" binding:"required"`
}

type CreateJournalLineRequest struct {
	AccountID   string `json:"account_id" binding:"required"`
	Debit       string `json:"debit"`
	Credit      string `json:"credit"`
	Description string `json:"description"`
}

type ReverseJournalEntryRequest struct {
	ReversalDate string `json:"reversal_date"`
	Reason       string `json:"reason"`
}

type TrialBalanceQuery struct {
	AsOfDate string `form:"as_of_date" binding:"required"`
}

type GeneralLedgerQuery struct {
	StartDate string `form:"start_date" binding:"required"`
	EndDate   string `form:"end_date" binding:"required"`
	AccountID string `form:"account_id"`
}

type AccountLedgerQuery struct {
	StartDate string `form:"start_date" binding:"required"`
	EndDate   string `form:"end_date" binding:"required"`
}

type ProfitLossQuery struct {
	StartDate string `form:"start_date" binding:"required"`
	EndDate   string `form:"end_date" binding:"required"`
}

type BalanceSheetQuery struct {
	AsOfDate string `form:"as_of_date" binding:"required"`
}

type CreateCashBankAccountRequest struct {
	AccountID         string `json:"account_id" binding:"required"`
	Type              string `json:"type" binding:"required"`
	BankName          string `json:"bank_name"`
	AccountNumber     string `json:"account_number"`
	AccountHolderName string `json:"account_holder_name"`
}

type UpdateCashBankAccountRequest struct {
	BankName          string `json:"bank_name"`
	AccountNumber     string `json:"account_number"`
	AccountHolderName string `json:"account_holder_name"`
	IsActive          bool   `json:"is_active"`
}

type CreateCashTransactionRequest struct {
	CashBankAccountID        string `json:"cash_bank_account_id" binding:"required"`
	TransactionDate          string `json:"transaction_date" binding:"required"`
	TransactionType          string `json:"transaction_type" binding:"required"`
	Amount                   string `json:"amount" binding:"required"`
	CounterCashBankAccountID string `json:"counter_cash_bank_account_id"`
	ContraAccountID          string `json:"contra_account_id"`
	Reference                string `json:"reference"`
	Description              string `json:"description"`
}

type CashTransactionListQuery struct {
	CashBankAccountID string `form:"cash_bank_account_id"`
	StartDate         string `form:"start_date"`
	EndDate           string `form:"end_date"`
	Page              int    `form:"page"`
	PerPage           int    `form:"per_page"`
}

type CreateBankReconciliationRequest struct {
	CashBankAccountID      string `json:"cash_bank_account_id" binding:"required"`
	StatementDate          string `json:"statement_date" binding:"required"`
	StatementEndingBalance string `json:"statement_ending_balance" binding:"required"`
	Notes                  string `json:"notes"`
}
