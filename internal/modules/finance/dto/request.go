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

type CreateBusinessPartnerRequest struct {
	PartnerType      string `json:"partner_type" binding:"required"`
	Code             string `json:"code" binding:"required"`
	Name             string `json:"name" binding:"required"`
	TaxID            string `json:"tax_id"`
	Address          string `json:"address"`
	ControlAccountID string `json:"control_account_id" binding:"required"`
}

type UpdateBusinessPartnerRequest struct {
	Name     string `json:"name" binding:"required"`
	TaxID    string `json:"tax_id"`
	Address  string `json:"address"`
	IsActive bool   `json:"is_active"`
}

type BusinessPartnerListQuery struct {
	PartnerType     string `form:"partner_type"`
	IncludeInactive bool   `form:"include_inactive"`
}

type CreateARAPTransactionRequest struct {
	PartnerID       string `json:"partner_id" binding:"required"`
	TransactionType string `json:"transaction_type" binding:"required"`
	TransactionDate string `json:"transaction_date" binding:"required"`
	DueDate         string `json:"due_date" binding:"required"`
	ReferenceNumber string `json:"reference_number"`
	Amount          string `json:"amount" binding:"required"`
	ContraAccountID string `json:"contra_account_id" binding:"required"`
	Description     string `json:"description"`
}

type ARAPTransactionListQuery struct {
	PartnerID       string `form:"partner_id"`
	TransactionType string `form:"transaction_type"`
	Status          string `form:"status"`
	Page            int    `form:"page"`
	PerPage         int    `form:"per_page"`
}

type CreateARAPPaymentRequest struct {
	PartnerID         string `json:"partner_id" binding:"required"`
	ARAPTransactionID string `json:"ar_ap_transaction_id" binding:"required"`
	PaymentDate       string `json:"payment_date" binding:"required"`
	Amount            string `json:"amount" binding:"required"`
	CashBankAccountID string `json:"cash_bank_account_id" binding:"required"`
	Notes             string `json:"notes"`
}

type AgingReportQuery struct {
	TransactionType string `form:"transaction_type" binding:"required"`
	AsOfDate        string `form:"as_of_date" binding:"required"`
}

type CreateAssetCategoryRequest struct {
	Code                             string `json:"code" binding:"required"`
	Name                             string `json:"name" binding:"required"`
	AssetAccountID                   string `json:"asset_account_id" binding:"required"`
	AccumulatedDepreciationAccountID string `json:"accumulated_depreciation_account_id" binding:"required"`
	DepreciationExpenseAccountID     string `json:"depreciation_expense_account_id" binding:"required"`
	DefaultUsefulLifeMonths          *int   `json:"default_useful_life_months"`
}

type AssetCategoryListQuery struct {
	IncludeInactive bool `form:"include_inactive"`
}

type CreateFixedAssetRequest struct {
	AssetCategoryID  string `json:"asset_category_id" binding:"required"`
	AssetCode        string `json:"asset_code" binding:"required"`
	AssetName        string `json:"asset_name" binding:"required"`
	AcquisitionDate  string `json:"acquisition_date" binding:"required"`
	AcquisitionCost  string `json:"acquisition_cost" binding:"required"`
	SalvageValue     string `json:"salvage_value"`
	UsefulLifeMonths int    `json:"useful_life_months" binding:"required"`
	Description      string `json:"description"`
	ContraAccountID  string `json:"contra_account_id" binding:"required"`
}

type FixedAssetListQuery struct {
	AssetCategoryID string `form:"asset_category_id"`
	Status          string `form:"status"`
	Page            int    `form:"page"`
	PerPage         int    `form:"per_page"`
}

type PostDepreciationRequest struct {
	AsOfDate string `json:"as_of_date" binding:"required"`
}
