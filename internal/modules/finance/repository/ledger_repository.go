package repository

import (
	"context"
	"time"

	"zyad.cloud/internal/platform/database"
)

type LedgerRepository struct {
	db *database.Pool
}

func NewLedgerRepository(db *database.Pool) *LedgerRepository {
	return &LedgerRepository{db: db}
}

type TrialBalanceRow struct {
	AccountID   string
	AccountCode string
	AccountName string
	Debit       string
	Credit      string
}

// TrialBalance returns, per postable account, the raw sum of posted debits
// and credits up to and including asOf, plus the account's opening balance
// (if effective by asOf). Grand totals of the returned Debit/Credit columns
// tie out by construction, since every posted journal entry is balanced.
func (r *LedgerRepository) TrialBalance(ctx context.Context, asOf time.Time) ([]TrialBalanceRow, error) {
	rows, err := r.db.Query(ctx, `
		WITH postings AS (
			SELECT jl.account_id, SUM(jl.debit) AS debit, SUM(jl.credit) AS credit
			FROM finance_journal_lines jl
			JOIN finance_journal_entries je ON je.id = jl.journal_entry_id
			WHERE je.status = 'posted' AND je.entry_date <= $1
			GROUP BY jl.account_id
		),
		opening AS (
			SELECT id AS account_id,
				CASE WHEN normal_balance = 'debit' THEN opening_balance ELSE 0 END AS debit,
				CASE WHEN normal_balance = 'credit' THEN opening_balance ELSE 0 END AS credit
			FROM finance_accounts
			WHERE opening_balance_date IS NOT NULL AND opening_balance_date <= $1 AND opening_balance <> 0
		)
		SELECT a.id, a.account_code, a.account_name,
			(COALESCE(p.debit, 0) + COALESCE(o.debit, 0))::text,
			(COALESCE(p.credit, 0) + COALESCE(o.credit, 0))::text
		FROM finance_accounts a
		LEFT JOIN postings p ON p.account_id = a.id
		LEFT JOIN opening o ON o.account_id = a.id
		WHERE a.is_header = false AND a.deleted_at IS NULL
			AND (p.account_id IS NOT NULL OR o.account_id IS NOT NULL)
		ORDER BY a.account_code ASC
	`, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]TrialBalanceRow, 0)
	for rows.Next() {
		var row TrialBalanceRow
		if err := rows.Scan(&row.AccountID, &row.AccountCode, &row.AccountName, &row.Debit, &row.Credit); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

type ReportAccountRow struct {
	AccountID     string
	AccountCode   string
	AccountName   string
	ReportSection string
	CategoryName  string
	CategorySort  int
	Amount        string
}

// ProfitLoss returns, per account under a profit_loss-statement category,
// the net movement between start and end (inclusive), sign-normalized so a
// positive amount always means "more revenue" / "more expense" per the
// account's own normal balance.
func (r *LedgerRepository) ProfitLoss(ctx context.Context, start, end time.Time) ([]ReportAccountRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT a.id, a.account_code, a.account_name, cat.report_section, cat.name, cat.sort_order,
			(SUM(
				CASE WHEN a.normal_balance = 'credit' THEN jl.credit - jl.debit ELSE jl.debit - jl.credit END
			))::text AS amount
		FROM finance_journal_lines jl
		JOIN finance_journal_entries je ON je.id = jl.journal_entry_id
		JOIN finance_accounts a ON a.id = jl.account_id
		JOIN finance_account_categories cat ON cat.id = a.account_category_id
		JOIN finance_account_types t ON t.id = cat.account_type_id
		WHERE je.status = 'posted'
			AND je.entry_date BETWEEN $1 AND $2
			AND t.financial_statement = 'profit_loss'
		GROUP BY a.id, a.account_code, a.account_name, cat.report_section, cat.name, cat.sort_order
		ORDER BY cat.sort_order ASC, a.account_code ASC
	`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanReportAccountRows(rows)
}

// BalanceSheet returns, per account under a balance_sheet-statement
// category, the cumulative balance as of asOf (postings plus opening
// balance), sign-normalized to the account's own normal balance.
func (r *LedgerRepository) BalanceSheet(ctx context.Context, asOf time.Time) ([]ReportAccountRow, error) {
	rows, err := r.db.Query(ctx, `
		WITH postings AS (
			SELECT jl.account_id,
				SUM(CASE WHEN a.normal_balance = 'debit' THEN jl.debit - jl.credit ELSE jl.credit - jl.debit END) AS amount
			FROM finance_journal_lines jl
			JOIN finance_journal_entries je ON je.id = jl.journal_entry_id
			JOIN finance_accounts a ON a.id = jl.account_id
			WHERE je.status = 'posted' AND je.entry_date <= $1
			GROUP BY jl.account_id
		),
		opening AS (
			SELECT id AS account_id, opening_balance AS amount
			FROM finance_accounts
			WHERE opening_balance_date IS NOT NULL AND opening_balance_date <= $1 AND opening_balance <> 0
		)
		SELECT a.id, a.account_code, a.account_name, cat.report_section, cat.name, cat.sort_order,
			(COALESCE(p.amount, 0) + COALESCE(o.amount, 0))::text AS amount
		FROM finance_accounts a
		JOIN finance_account_categories cat ON cat.id = a.account_category_id
		JOIN finance_account_types t ON t.id = cat.account_type_id
		LEFT JOIN postings p ON p.account_id = a.id
		LEFT JOIN opening o ON o.account_id = a.id
		WHERE a.is_header = false AND a.deleted_at IS NULL
			AND t.financial_statement = 'balance_sheet'
			AND (p.account_id IS NOT NULL OR o.account_id IS NOT NULL)
		ORDER BY cat.sort_order ASC, a.account_code ASC
	`, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanReportAccountRows(rows)
}

func scanReportAccountRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]ReportAccountRow, error) {
	result := make([]ReportAccountRow, 0)
	for rows.Next() {
		var row ReportAccountRow
		if err := rows.Scan(&row.AccountID, &row.AccountCode, &row.AccountName, &row.ReportSection, &row.CategoryName, &row.CategorySort, &row.Amount); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

type LedgerLineRow struct {
	EntryDate   time.Time
	EntryNumber string
	AccountID   string
	AccountCode string
	AccountName string
	Description string
	Debit       string
	Credit      string
}

func (r *LedgerRepository) GeneralLedger(ctx context.Context, start, end time.Time, accountID string) ([]LedgerLineRow, error) {
	args := []any{start, end}
	where := ""
	if accountID != "" {
		args = append(args, accountID)
		where = " AND jl.account_id = $3::uuid"
	}
	rows, err := r.db.Query(ctx, `
		SELECT je.entry_date, je.entry_number, a.id, a.account_code, a.account_name,
			COALESCE(NULLIF(jl.description, ''), je.description), jl.debit::text, jl.credit::text
		FROM finance_journal_lines jl
		JOIN finance_journal_entries je ON je.id = jl.journal_entry_id
		JOIN finance_accounts a ON a.id = jl.account_id
		WHERE je.status = 'posted' AND je.entry_date BETWEEN $1 AND $2`+where+`
		ORDER BY je.entry_date ASC, je.entry_number ASC, jl.line_number ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]LedgerLineRow, 0)
	for rows.Next() {
		var row LedgerLineRow
		if err := rows.Scan(&row.EntryDate, &row.EntryNumber, &row.AccountID, &row.AccountCode, &row.AccountName, &row.Description, &row.Debit, &row.Credit); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// AccountBalanceAsOf returns the account's normal-balance-signed cumulative
// balance up to and including asOf (postings + opening balance).
func (r *LedgerRepository) AccountBalanceAsOf(ctx context.Context, accountID string, asOf time.Time) (string, error) {
	var amount string
	err := r.db.QueryRow(ctx, `
		WITH postings AS (
			SELECT SUM(CASE WHEN a.normal_balance = 'debit' THEN jl.debit - jl.credit ELSE jl.credit - jl.debit END) AS amount
			FROM finance_journal_lines jl
			JOIN finance_journal_entries je ON je.id = jl.journal_entry_id
			JOIN finance_accounts a ON a.id = jl.account_id
			WHERE je.status = 'posted' AND je.entry_date <= $2 AND jl.account_id = $1::uuid
		),
		opening AS (
			SELECT opening_balance AS amount
			FROM finance_accounts
			WHERE id = $1::uuid AND opening_balance_date IS NOT NULL AND opening_balance_date <= $2
		)
		SELECT (COALESCE((SELECT amount FROM postings), 0) + COALESCE((SELECT amount FROM opening), 0))::text
	`, accountID, asOf).Scan(&amount)
	if err != nil {
		return "", err
	}
	return amount, nil
}

// AccountOpeningBalanceBefore returns the account's normal-balance-signed
// cumulative balance strictly before `before` (postings + opening balance),
// used as the starting point for an account ledger's running balance.
func (r *LedgerRepository) AccountOpeningBalanceBefore(ctx context.Context, accountID string, before time.Time) (string, error) {
	var amount string
	err := r.db.QueryRow(ctx, `
		WITH postings AS (
			SELECT SUM(CASE WHEN a.normal_balance = 'debit' THEN jl.debit - jl.credit ELSE jl.credit - jl.debit END) AS amount
			FROM finance_journal_lines jl
			JOIN finance_journal_entries je ON je.id = jl.journal_entry_id
			JOIN finance_accounts a ON a.id = jl.account_id
			WHERE je.status = 'posted' AND je.entry_date < $2 AND jl.account_id = $1::uuid
		),
		opening AS (
			SELECT opening_balance AS amount
			FROM finance_accounts
			WHERE id = $1::uuid AND opening_balance_date IS NOT NULL AND opening_balance_date < $2
		)
		SELECT (COALESCE((SELECT amount FROM postings), 0) + COALESCE((SELECT amount FROM opening), 0))::text
	`, accountID, before).Scan(&amount)
	if err != nil {
		return "", err
	}
	return amount, nil
}
