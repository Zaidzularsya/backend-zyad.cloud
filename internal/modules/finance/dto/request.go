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
