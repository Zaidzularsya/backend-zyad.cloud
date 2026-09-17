package repository

import (
	"context"
	"strings"
	"time"

	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/platform/database"
)

const accountSelectColumns = `
	a.id,
	a.account_code,
	a.account_name,
	a.account_category_id,
	COALESCE(c.code, ''),
	COALESCE(c.name, ''),
	COALESCE(t.code, ''),
	a.parent_account_id,
	a.is_header,
	a.normal_balance,
	a.is_active,
	a.opening_balance::text,
	a.opening_balance_date,
	COALESCE(a.description, ''),
	a.created_at,
	a.updated_at,
	a.deleted_at
`

const accountFrom = `
	FROM finance_accounts a
	LEFT JOIN finance_account_categories c ON c.id = a.account_category_id
	LEFT JOIN finance_account_types t ON t.id = c.account_type_id
`

type AccountRepository struct {
	db *database.Pool
}

func NewAccountRepository(db *database.Pool) *AccountRepository {
	return &AccountRepository{db: db}
}

type CreateAccountParams struct {
	AccountCode        string
	AccountName        string
	AccountCategoryID  *string
	ParentAccountID    *string
	IsHeader           bool
	NormalBalance      string
	OpeningBalance     string
	OpeningBalanceDate *time.Time
	Description        string
}

type UpdateAccountParams struct {
	ID                 string
	AccountName        string
	AccountCategoryID  *string
	ParentAccountID    *string
	IsActive           bool
	OpeningBalance     string
	OpeningBalanceDate *time.Time
	Description        string
}

type AccountListFilter struct {
	IncludeInactive bool
	CategoryID      string
}

func (r *AccountRepository) ListAccountTypes(ctx context.Context) ([]model.AccountType, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, code, name, normal_balance, financial_statement, sort_order, created_at, updated_at
		FROM finance_account_types
		ORDER BY sort_order ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	types := make([]model.AccountType, 0)
	for rows.Next() {
		var t model.AccountType
		if err := rows.Scan(&t.ID, &t.Code, &t.Name, &t.NormalBalance, &t.FinancialStatement, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	return types, rows.Err()
}

func (r *AccountRepository) ListAccountCategories(ctx context.Context) ([]model.AccountCategory, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, account_type_id, code, name, report_section, sort_order, created_at, updated_at
		FROM finance_account_categories
		ORDER BY sort_order ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := make([]model.AccountCategory, 0)
	for rows.Next() {
		var c model.AccountCategory
		if err := rows.Scan(&c.ID, &c.AccountTypeID, &c.Code, &c.Name, &c.ReportSection, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, rows.Err()
}

func (r *AccountRepository) Create(ctx context.Context, params CreateAccountParams) (model.Account, error) {
	var account model.Account
	err := r.db.QueryRow(ctx, `
		INSERT INTO finance_accounts (
			account_code, account_name, account_category_id, parent_account_id,
			is_header, normal_balance, opening_balance, opening_balance_date, description
		)
		VALUES ($1, $2, $3::uuid, $4::uuid, $5, $6, $7::numeric, $8, NULLIF($9, ''))
		RETURNING id, account_code, account_name, account_category_id, ''::text, ''::text, ''::text,
			parent_account_id, is_header, normal_balance, is_active, opening_balance::text,
			opening_balance_date, COALESCE(description, ''), created_at, updated_at, deleted_at
	`,
		strings.TrimSpace(params.AccountCode),
		strings.TrimSpace(params.AccountName),
		nullableUUID(params.AccountCategoryID),
		nullableUUID(params.ParentAccountID),
		params.IsHeader,
		params.NormalBalance,
		amountOrZero(params.OpeningBalance),
		params.OpeningBalanceDate,
		params.Description,
	).Scan(accountScanDest(&account)...)
	if err != nil {
		return model.Account{}, err
	}
	return account, nil
}

func (r *AccountRepository) Update(ctx context.Context, params UpdateAccountParams) (model.Account, error) {
	var account model.Account
	err := r.db.QueryRow(ctx, `
		UPDATE finance_accounts
		SET
			account_name = $2,
			account_category_id = $3::uuid,
			parent_account_id = $4::uuid,
			is_active = $5,
			opening_balance = $6::numeric,
			opening_balance_date = $7,
			description = NULLIF($8, ''),
			updated_at = now()
		WHERE id = $1::uuid AND deleted_at IS NULL
		RETURNING id, account_code, account_name, account_category_id, ''::text, ''::text, ''::text,
			parent_account_id, is_header, normal_balance, is_active, opening_balance::text,
			opening_balance_date, COALESCE(description, ''), created_at, updated_at, deleted_at
	`,
		strings.TrimSpace(params.ID),
		strings.TrimSpace(params.AccountName),
		nullableUUID(params.AccountCategoryID),
		nullableUUID(params.ParentAccountID),
		params.IsActive,
		amountOrZero(params.OpeningBalance),
		params.OpeningBalanceDate,
		params.Description,
	).Scan(accountScanDest(&account)...)
	if err != nil {
		return model.Account{}, err
	}
	return account, nil
}

func (r *AccountRepository) FindByID(ctx context.Context, id string) (model.Account, error) {
	var account model.Account
	err := r.db.QueryRow(ctx, `
		SELECT `+accountSelectColumns+accountFrom+`
		WHERE a.id = $1::uuid AND a.deleted_at IS NULL
	`, strings.TrimSpace(id)).Scan(accountScanDest(&account)...)
	if err != nil {
		return model.Account{}, err
	}
	return account, nil
}

func (r *AccountRepository) List(ctx context.Context, filter AccountListFilter) ([]model.Account, error) {
	where := " WHERE a.deleted_at IS NULL"
	args := []any{}
	if !filter.IncludeInactive {
		where += " AND a.is_active = true"
	}
	if filter.CategoryID != "" {
		args = append(args, strings.TrimSpace(filter.CategoryID))
		where += " AND a.account_category_id = $1::uuid"
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+accountSelectColumns+accountFrom+where+`
		ORDER BY a.account_code ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]model.Account, 0)
	for rows.Next() {
		var account model.Account
		if err := rows.Scan(accountScanDest(&account)...); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func accountScanDest(account *model.Account) []any {
	return []any{
		&account.ID,
		&account.AccountCode,
		&account.AccountName,
		&account.AccountCategoryID,
		&account.AccountCategoryCode,
		&account.AccountCategoryName,
		&account.AccountTypeCode,
		&account.ParentAccountID,
		&account.IsHeader,
		&account.NormalBalance,
		&account.IsActive,
		&account.OpeningBalance,
		&account.OpeningBalanceDate,
		&account.Description,
		&account.CreatedAt,
		&account.UpdatedAt,
		&account.DeletedAt,
	}
}
