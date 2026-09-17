package dto

type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type AccountTypeResponse struct {
	ID                 string `json:"id"`
	Code               string `json:"code"`
	Name               string `json:"name"`
	NormalBalance      string `json:"normal_balance"`
	FinancialStatement string `json:"financial_statement"`
	SortOrder          int    `json:"sort_order"`
}

type AccountCategoryResponse struct {
	ID            string `json:"id"`
	AccountTypeID string `json:"account_type_id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	ReportSection string `json:"report_section"`
	SortOrder     int    `json:"sort_order"`
}

type AccountResponse struct {
	ID                  string  `json:"id"`
	AccountCode         string  `json:"account_code"`
	AccountName         string  `json:"account_name"`
	AccountCategoryID   *string `json:"account_category_id,omitempty"`
	AccountCategoryCode string  `json:"account_category_code,omitempty"`
	AccountCategoryName string  `json:"account_category_name,omitempty"`
	AccountTypeCode     string  `json:"account_type_code,omitempty"`
	ParentAccountID     *string `json:"parent_account_id,omitempty"`
	IsHeader            bool    `json:"is_header"`
	NormalBalance       string  `json:"normal_balance"`
	IsActive            bool    `json:"is_active"`
	OpeningBalance      string  `json:"opening_balance"`
	OpeningBalanceDate  *string `json:"opening_balance_date,omitempty"`
	Description         string  `json:"description,omitempty"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
}

type FiscalYearResponse struct {
	ID        string                 `json:"id"`
	Year      int                    `json:"year"`
	StartDate string                 `json:"start_date"`
	EndDate   string                 `json:"end_date"`
	Status    string                 `json:"status"`
	ClosedAt  *string                `json:"closed_at,omitempty"`
	Periods   []FiscalPeriodResponse `json:"periods,omitempty"`
}

type FiscalPeriodResponse struct {
	ID           string  `json:"id"`
	FiscalYearID string  `json:"fiscal_year_id"`
	PeriodNumber int     `json:"period_number"`
	StartDate    string  `json:"start_date"`
	EndDate      string  `json:"end_date"`
	Status       string  `json:"status"`
	ClosedAt     *string `json:"closed_at,omitempty"`
}

type JournalLineResponse struct {
	ID            string  `json:"id"`
	LineNumber    int     `json:"line_number"`
	AccountID     string  `json:"account_id"`
	AccountCode   string  `json:"account_code,omitempty"`
	AccountName   string  `json:"account_name,omitempty"`
	Debit         string  `json:"debit"`
	Credit        string  `json:"credit"`
	Description   string  `json:"description,omitempty"`
	SubledgerType string  `json:"subledger_type"`
	SubledgerID   *string `json:"subledger_id,omitempty"`
}

type JournalEntryResponse struct {
	ID                string                `json:"id"`
	EntryNumber       string                `json:"entry_number"`
	EntryDate         string                `json:"entry_date"`
	FiscalPeriodID    string                `json:"fiscal_period_id"`
	SourceType        string                `json:"source_type"`
	SourceID          *string               `json:"source_id,omitempty"`
	Reference         string                `json:"reference,omitempty"`
	Description       string                `json:"description,omitempty"`
	Status            string                `json:"status"`
	PostedAt          *string               `json:"posted_at,omitempty"`
	ReversedByEntryID *string               `json:"reversed_by_entry_id,omitempty"`
	Lines             []JournalLineResponse `json:"lines,omitempty"`
	TotalDebit        string                `json:"total_debit"`
	TotalCredit       string                `json:"total_credit"`
	CreatedAt         string                `json:"created_at"`
	UpdatedAt         string                `json:"updated_at"`
}

type JournalEntryListResponse struct {
	Items []JournalEntryResponse `json:"items"`
	Meta  PaginationMeta         `json:"meta"`
}

type TrialBalanceLineResponse struct {
	AccountID   string `json:"account_id"`
	AccountCode string `json:"account_code"`
	AccountName string `json:"account_name"`
	Debit       string `json:"debit"`
	Credit      string `json:"credit"`
}

type TrialBalanceResponse struct {
	AsOfDate    string                     `json:"as_of_date"`
	Lines       []TrialBalanceLineResponse `json:"lines"`
	TotalDebit  string                     `json:"total_debit"`
	TotalCredit string                     `json:"total_credit"`
	IsBalanced  bool                       `json:"is_balanced"`
}

type ReportLineResponse struct {
	AccountID   string `json:"account_id"`
	AccountCode string `json:"account_code"`
	AccountName string `json:"account_name"`
	Amount      string `json:"amount"`
}

type ReportSectionResponse struct {
	ReportSection string               `json:"report_section"`
	Label         string               `json:"label"`
	Accounts      []ReportLineResponse `json:"accounts"`
	Subtotal      string               `json:"subtotal"`
}

type ProfitLossResponse struct {
	StartDate string                  `json:"start_date"`
	EndDate   string                  `json:"end_date"`
	Sections  []ReportSectionResponse `json:"sections"`
	NetIncome string                  `json:"net_income"`
}

type BalanceSheetResponse struct {
	AsOfDate             string                  `json:"as_of_date"`
	AssetSections        []ReportSectionResponse `json:"asset_sections"`
	LiabilitySections    []ReportSectionResponse `json:"liability_sections"`
	EquitySections       []ReportSectionResponse `json:"equity_sections"`
	CurrentYearNetIncome string                  `json:"current_year_net_income"`
	TotalAssets          string                  `json:"total_assets"`
	TotalLiabilities     string                  `json:"total_liabilities"`
	TotalEquity          string                  `json:"total_equity"`
	IsBalanced           bool                    `json:"is_balanced"`
}

type GeneralLedgerLineResponse struct {
	EntryDate   string `json:"entry_date"`
	EntryNumber string `json:"entry_number"`
	AccountID   string `json:"account_id"`
	AccountCode string `json:"account_code"`
	AccountName string `json:"account_name"`
	Description string `json:"description,omitempty"`
	Debit       string `json:"debit"`
	Credit      string `json:"credit"`
}

type GeneralLedgerResponse struct {
	StartDate string                      `json:"start_date"`
	EndDate   string                      `json:"end_date"`
	Lines     []GeneralLedgerLineResponse `json:"lines"`
}

type AccountLedgerLineResponse struct {
	EntryDate      string `json:"entry_date"`
	EntryNumber    string `json:"entry_number"`
	Description    string `json:"description,omitempty"`
	Debit          string `json:"debit"`
	Credit         string `json:"credit"`
	RunningBalance string `json:"running_balance"`
}

type CashBankAccountResponse struct {
	ID                string `json:"id"`
	AccountID         string `json:"account_id"`
	AccountCode       string `json:"account_code"`
	AccountName       string `json:"account_name"`
	Type              string `json:"type"`
	BankName          string `json:"bank_name,omitempty"`
	AccountNumber     string `json:"account_number,omitempty"`
	AccountHolderName string `json:"account_holder_name,omitempty"`
	Currency          string `json:"currency"`
	IsActive          bool   `json:"is_active"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

type CashTransactionResponse struct {
	ID                       string  `json:"id"`
	CashBankAccountID        string  `json:"cash_bank_account_id"`
	CashBankAccountLabel     string  `json:"cash_bank_account_label,omitempty"`
	TransactionDate          string  `json:"transaction_date"`
	TransactionType          string  `json:"transaction_type"`
	Amount                   string  `json:"amount"`
	CounterCashBankAccountID *string `json:"counter_cash_bank_account_id,omitempty"`
	CounterCashBankLabel     string  `json:"counter_cash_bank_label,omitempty"`
	ContraAccountID          *string `json:"contra_account_id,omitempty"`
	ContraAccountCode        string  `json:"contra_account_code,omitempty"`
	ContraAccountName        string  `json:"contra_account_name,omitempty"`
	Reference                string  `json:"reference,omitempty"`
	Description              string  `json:"description,omitempty"`
	JournalEntryID           string  `json:"journal_entry_id"`
	ReconciledAt             *string `json:"reconciled_at,omitempty"`
	CreatedAt                string  `json:"created_at"`
	UpdatedAt                string  `json:"updated_at"`
}

type CashTransactionListResponse struct {
	Items []CashTransactionResponse `json:"items"`
	Meta  PaginationMeta            `json:"meta"`
}

type BankReconciliationResponse struct {
	ID                     string  `json:"id"`
	CashBankAccountID      string  `json:"cash_bank_account_id"`
	StatementDate          string  `json:"statement_date"`
	StatementEndingBalance string  `json:"statement_ending_balance"`
	BookEndingBalance      string  `json:"book_ending_balance"`
	Difference             string  `json:"difference"`
	Status                 string  `json:"status"`
	Notes                  string  `json:"notes,omitempty"`
	CompletedAt            *string `json:"completed_at,omitempty"`
	CreatedAt              string  `json:"created_at"`
}

type BusinessPartnerResponse struct {
	ID                 string `json:"id"`
	PartnerType        string `json:"partner_type"`
	Code               string `json:"code"`
	Name               string `json:"name"`
	TaxID              string `json:"tax_id,omitempty"`
	Address            string `json:"address,omitempty"`
	ControlAccountID   string `json:"control_account_id"`
	ControlAccountCode string `json:"control_account_code,omitempty"`
	ControlAccountName string `json:"control_account_name,omitempty"`
	IsActive           bool   `json:"is_active"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type ARAPTransactionResponse struct {
	ID                string `json:"id"`
	PartnerID         string `json:"partner_id"`
	PartnerCode       string `json:"partner_code,omitempty"`
	PartnerName       string `json:"partner_name,omitempty"`
	TransactionType   string `json:"transaction_type"`
	TransactionDate   string `json:"transaction_date"`
	DueDate           string `json:"due_date"`
	ReferenceNumber   string `json:"reference_number,omitempty"`
	Amount            string `json:"amount"`
	ContraAccountID   string `json:"contra_account_id"`
	ContraAccountCode string `json:"contra_account_code,omitempty"`
	ContraAccountName string `json:"contra_account_name,omitempty"`
	Description       string `json:"description,omitempty"`
	Status            string `json:"status"`
	JournalEntryID    string `json:"journal_entry_id"`
	PaidAmount        string `json:"paid_amount"`
	OutstandingAmount string `json:"outstanding_amount"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

type ARAPTransactionListResponse struct {
	Items []ARAPTransactionResponse `json:"items"`
	Meta  PaginationMeta            `json:"meta"`
}

type ARAPPaymentResponse struct {
	ID                   string `json:"id"`
	PartnerID            string `json:"partner_id"`
	PaymentDate          string `json:"payment_date"`
	Amount               string `json:"amount"`
	CashBankAccountID    string `json:"cash_bank_account_id"`
	CashBankAccountLabel string `json:"cash_bank_account_label,omitempty"`
	JournalEntryID       string `json:"journal_entry_id"`
	Notes                string `json:"notes,omitempty"`
	CreatedAt            string `json:"created_at"`
}

type AgingBucketResponse struct {
	Label  string `json:"label"`
	Amount string `json:"amount"`
}

type AgingRowResponse struct {
	PartnerID   string                `json:"partner_id"`
	PartnerCode string                `json:"partner_code"`
	PartnerName string                `json:"partner_name"`
	Buckets     []AgingBucketResponse `json:"buckets"`
	Total       string                `json:"total"`
}

type AgingReportResponse struct {
	AsOfDate   string             `json:"as_of_date"`
	Rows       []AgingRowResponse `json:"rows"`
	GrandTotal string             `json:"grand_total"`
}

type AccountLedgerResponse struct {
	AccountID      string                      `json:"account_id"`
	AccountCode    string                      `json:"account_code"`
	AccountName    string                      `json:"account_name"`
	StartDate      string                      `json:"start_date"`
	EndDate        string                      `json:"end_date"`
	OpeningBalance string                      `json:"opening_balance"`
	Lines          []AccountLedgerLineResponse `json:"lines"`
	ClosingBalance string                      `json:"closing_balance"`
}
