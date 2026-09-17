package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/platform/database"
)

const cashBankAccountSelectColumns = `
	cba.id,
	cba.account_id,
	a.account_code,
	a.account_name,
	cba.type,
	COALESCE(cba.bank_name, ''),
	COALESCE(cba.account_number, ''),
	COALESCE(cba.account_holder_name, ''),
	cba.currency,
	cba.is_active,
	cba.created_at,
	cba.updated_at
`

const cashBankAccountFrom = `
	FROM finance_cash_bank_accounts cba
	JOIN finance_accounts a ON a.id = cba.account_id
`

type CashBankRepository struct {
	db *database.Pool
}

func NewCashBankRepository(db *database.Pool) *CashBankRepository {
	return &CashBankRepository{db: db}
}

type CreateCashBankAccountParams struct {
	AccountID         string
	Type              model.CashBankAccountType
	BankName          string
	AccountNumber     string
	AccountHolderName string
}

type UpdateCashBankAccountParams struct {
	ID                string
	BankName          string
	AccountNumber     string
	AccountHolderName string
	IsActive          bool
}

func (r *CashBankRepository) Create(ctx context.Context, params CreateCashBankAccountParams) (model.CashBankAccount, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO finance_cash_bank_accounts (account_id, type, bank_name, account_number, account_holder_name)
		VALUES ($1::uuid, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''))
		RETURNING id
	`, strings.TrimSpace(params.AccountID), string(params.Type), params.BankName, params.AccountNumber, params.AccountHolderName).
		Scan(&id)
	if err != nil {
		return model.CashBankAccount{}, err
	}
	return r.FindByID(ctx, id)
}

func (r *CashBankRepository) Update(ctx context.Context, params UpdateCashBankAccountParams) (model.CashBankAccount, error) {
	_, err := r.db.Exec(ctx, `
		UPDATE finance_cash_bank_accounts
		SET bank_name = NULLIF($2, ''), account_number = NULLIF($3, ''),
			account_holder_name = NULLIF($4, ''), is_active = $5, updated_at = now()
		WHERE id = $1::uuid
	`, strings.TrimSpace(params.ID), params.BankName, params.AccountNumber, params.AccountHolderName, params.IsActive)
	if err != nil {
		return model.CashBankAccount{}, err
	}
	return r.FindByID(ctx, params.ID)
}

func (r *CashBankRepository) FindByID(ctx context.Context, id string) (model.CashBankAccount, error) {
	var account model.CashBankAccount
	err := r.db.QueryRow(ctx, `
		SELECT `+cashBankAccountSelectColumns+cashBankAccountFrom+`
		WHERE cba.id = $1::uuid
	`, strings.TrimSpace(id)).Scan(cashBankAccountScanDest(&account)...)
	if err != nil {
		return model.CashBankAccount{}, err
	}
	return account, nil
}

func (r *CashBankRepository) List(ctx context.Context, includeInactive bool) ([]model.CashBankAccount, error) {
	where := ""
	if !includeInactive {
		where = " WHERE cba.is_active = true"
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+cashBankAccountSelectColumns+cashBankAccountFrom+where+`
		ORDER BY a.account_code ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]model.CashBankAccount, 0)
	for rows.Next() {
		var account model.CashBankAccount
		if err := rows.Scan(cashBankAccountScanDest(&account)...); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func cashBankAccountScanDest(account *model.CashBankAccount) []any {
	return []any{
		&account.ID,
		&account.AccountID,
		&account.AccountCode,
		&account.AccountName,
		&account.Type,
		&account.BankName,
		&account.AccountNumber,
		&account.AccountHolderName,
		&account.Currency,
		&account.IsActive,
		&account.CreatedAt,
		&account.UpdatedAt,
	}
}

type CreateCashTransactionParams struct {
	CashBankAccountID        string
	TransactionDate          time.Time
	TransactionType          model.CashTransactionType
	Amount                   string
	CounterCashBankAccountID *string
	ContraAccountID          *string
	Reference                string
	Description              string
	EntryNumber              string
	CreatedBy                *string
}

// CreateTransaction posts the underlying journal entry (Dr/Cr resolved from
// the transaction type) and inserts the cash_transactions row referencing it,
// all within a single DB transaction, so a cash movement can never exist
// without its GL posting or vice versa.
func (r *CashBankRepository) CreateTransaction(ctx context.Context, params CreateCashTransactionParams) (model.CashTransaction, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.CashTransaction{}, err
	}
	defer tx.Rollback(ctx)

	var thisAccountGLID string
	if err := tx.QueryRow(ctx, `SELECT account_id FROM finance_cash_bank_accounts WHERE id = $1::uuid`, params.CashBankAccountID).
		Scan(&thisAccountGLID); err != nil {
		return model.CashTransaction{}, err
	}

	var debitAccountID, creditAccountID string
	switch params.TransactionType {
	case model.CashTransactionTypeCashIn:
		debitAccountID = thisAccountGLID
		creditAccountID = *params.ContraAccountID
	case model.CashTransactionTypeCashOut:
		debitAccountID = *params.ContraAccountID
		creditAccountID = thisAccountGLID
	case model.CashTransactionTypeTransfer:
		var counterAccountGLID string
		if err := tx.QueryRow(ctx, `SELECT account_id FROM finance_cash_bank_accounts WHERE id = $1::uuid`, *params.CounterCashBankAccountID).
			Scan(&counterAccountGLID); err != nil {
			return model.CashTransaction{}, err
		}
		debitAccountID = counterAccountGLID
		creditAccountID = thisAccountGLID
	default:
		return model.CashTransaction{}, fmt.Errorf("finance: invalid cash transaction type %q", params.TransactionType)
	}

	entryID, err := InsertJournalEntryTx(ctx, tx, CreateJournalEntryParams{
		EntryNumber: params.EntryNumber,
		EntryDate:   params.TransactionDate,
		SourceType:  "cash_bank",
		Reference:   params.Reference,
		Description: params.Description,
		CreatedBy:   params.CreatedBy,
		Lines: []CreateJournalLineParams{
			{AccountID: debitAccountID, Debit: params.Amount},
			{AccountID: creditAccountID, Credit: params.Amount},
		},
	})
	if err != nil {
		return model.CashTransaction{}, err
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO finance_cash_transactions (
			cash_bank_account_id, transaction_date, transaction_type, amount,
			counter_cash_bank_account_id, contra_account_id, reference, description,
			journal_entry_id, created_by
		)
		VALUES ($1::uuid, $2, $3, $4::numeric, $5::uuid, $6::uuid, NULLIF($7, ''), NULLIF($8, ''), $9::uuid, $10::uuid)
		RETURNING id
	`,
		params.CashBankAccountID, params.TransactionDate, string(params.TransactionType), amountOrZero(params.Amount),
		nullableUUID(params.CounterCashBankAccountID), nullableUUID(params.ContraAccountID), params.Reference, params.Description,
		entryID, nullableUUID(params.CreatedBy),
	).Scan(&id)
	if err != nil {
		return model.CashTransaction{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.CashTransaction{}, err
	}
	return r.FindTransactionByID(ctx, id)
}

const cashTransactionSelectColumns = `
	ct.id,
	ct.cash_bank_account_id,
	a.account_code,
	a.account_name,
	ct.transaction_date,
	ct.transaction_type,
	ct.amount::text,
	ct.counter_cash_bank_account_id,
	COALESCE(ca.account_code, ''),
	ct.contra_account_id,
	COALESCE(contra.account_code, ''),
	COALESCE(contra.account_name, ''),
	COALESCE(ct.reference, ''),
	COALESCE(ct.description, ''),
	ct.journal_entry_id,
	ct.reconciled_at,
	ct.created_at,
	ct.updated_at
`

const cashTransactionFrom = `
	FROM finance_cash_transactions ct
	JOIN finance_cash_bank_accounts cba ON cba.id = ct.cash_bank_account_id
	JOIN finance_accounts a ON a.id = cba.account_id
	LEFT JOIN finance_cash_bank_accounts counter_cba ON counter_cba.id = ct.counter_cash_bank_account_id
	LEFT JOIN finance_accounts ca ON ca.id = counter_cba.account_id
	LEFT JOIN finance_accounts contra ON contra.id = ct.contra_account_id
`

func (r *CashBankRepository) FindTransactionByID(ctx context.Context, id string) (model.CashTransaction, error) {
	var t model.CashTransaction
	err := r.db.QueryRow(ctx, `
		SELECT `+cashTransactionSelectColumns+cashTransactionFrom+`
		WHERE ct.id = $1::uuid
	`, strings.TrimSpace(id)).Scan(cashTransactionScanDest(&t)...)
	if err != nil {
		return model.CashTransaction{}, err
	}
	return t, nil
}

type CashTransactionListFilter struct {
	CashBankAccountID string
	StartDate         *time.Time
	EndDate           *time.Time
	Limit             int
	Offset            int
}

func (r *CashBankRepository) ListTransactions(ctx context.Context, filter CashTransactionListFilter) ([]model.CashTransaction, int64, error) {
	conditions := []string{}
	args := []any{}
	if filter.CashBankAccountID != "" {
		args = append(args, filter.CashBankAccountID)
		conditions = append(conditions, fmt.Sprintf("ct.cash_bank_account_id = $%d::uuid", len(args)))
	}
	if filter.StartDate != nil {
		args = append(args, *filter.StartDate)
		conditions = append(conditions, fmt.Sprintf("ct.transaction_date >= $%d", len(args)))
	}
	if filter.EndDate != nil {
		args = append(args, *filter.EndDate)
		conditions = append(conditions, fmt.Sprintf("ct.transaction_date <= $%d", len(args)))
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) "+cashTransactionFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := pagination(filter.Limit, filter.Offset)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+cashTransactionSelectColumns+cashTransactionFrom+where+`
		ORDER BY ct.transaction_date DESC, ct.created_at DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	transactions := make([]model.CashTransaction, 0)
	for rows.Next() {
		var t model.CashTransaction
		if err := rows.Scan(cashTransactionScanDest(&t)...); err != nil {
			return nil, 0, err
		}
		transactions = append(transactions, t)
	}
	return transactions, total, rows.Err()
}

func cashTransactionScanDest(t *model.CashTransaction) []any {
	return []any{
		&t.ID,
		&t.CashBankAccountID,
		&t.CashBankAccountCode,
		&t.CashBankAccountLabel,
		&t.TransactionDate,
		&t.TransactionType,
		&t.Amount,
		&t.CounterCashBankAccountID,
		&t.CounterCashBankLabel,
		&t.ContraAccountID,
		&t.ContraAccountCode,
		&t.ContraAccountName,
		&t.Reference,
		&t.Description,
		&t.JournalEntryID,
		&t.ReconciledAt,
		&t.CreatedAt,
		&t.UpdatedAt,
	}
}

var ErrBankReconciliationNotFound = errors.New("finance: bank reconciliation not found")

type CreateBankReconciliationParams struct {
	CashBankAccountID      string
	StatementDate          time.Time
	StatementEndingBalance string
	BookEndingBalance      string
	Notes                  string
}

func (r *CashBankRepository) CreateReconciliation(ctx context.Context, params CreateBankReconciliationParams) (model.BankReconciliation, error) {
	var rec model.BankReconciliation
	err := r.db.QueryRow(ctx, `
		INSERT INTO finance_bank_reconciliations (
			cash_bank_account_id, statement_date, statement_ending_balance, book_ending_balance, notes
		)
		VALUES ($1::uuid, $2, $3::numeric, $4::numeric, NULLIF($5, ''))
		RETURNING id, cash_bank_account_id, statement_date, statement_ending_balance::text,
			book_ending_balance::text, status, COALESCE(notes, ''), completed_at, completed_by, created_at, updated_at
	`, params.CashBankAccountID, params.StatementDate, amountOrZero(params.StatementEndingBalance), amountOrZero(params.BookEndingBalance), params.Notes).
		Scan(&rec.ID, &rec.CashBankAccountID, &rec.StatementDate, &rec.StatementEndingBalance,
			&rec.BookEndingBalance, &rec.Status, &rec.Notes, &rec.CompletedAt, &rec.CompletedBy, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		return model.BankReconciliation{}, err
	}
	return rec, nil
}

func (r *CashBankRepository) ListReconciliations(ctx context.Context, cashBankAccountID string) ([]model.BankReconciliation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, cash_bank_account_id, statement_date, statement_ending_balance::text,
			book_ending_balance::text, status, COALESCE(notes, ''), completed_at, completed_by, created_at, updated_at
		FROM finance_bank_reconciliations
		WHERE cash_bank_account_id = $1::uuid
		ORDER BY statement_date DESC
	`, cashBankAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.BankReconciliation, 0)
	for rows.Next() {
		var rec model.BankReconciliation
		if err := rows.Scan(&rec.ID, &rec.CashBankAccountID, &rec.StatementDate, &rec.StatementEndingBalance,
			&rec.BookEndingBalance, &rec.Status, &rec.Notes, &rec.CompletedAt, &rec.CompletedBy, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, rec)
	}
	return items, rows.Err()
}

func (r *CashBankRepository) CompleteReconciliation(ctx context.Context, id string, actorID *string) (model.BankReconciliation, error) {
	var rec model.BankReconciliation
	err := r.db.QueryRow(ctx, `
		UPDATE finance_bank_reconciliations
		SET status = 'completed', completed_at = now(), completed_by = $2::uuid, updated_at = now()
		WHERE id = $1::uuid
		RETURNING id, cash_bank_account_id, statement_date, statement_ending_balance::text,
			book_ending_balance::text, status, COALESCE(notes, ''), completed_at, completed_by, created_at, updated_at
	`, strings.TrimSpace(id), nullableUUID(actorID)).
		Scan(&rec.ID, &rec.CashBankAccountID, &rec.StatementDate, &rec.StatementEndingBalance,
			&rec.BookEndingBalance, &rec.Status, &rec.Notes, &rec.CompletedAt, &rec.CompletedBy, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.BankReconciliation{}, ErrBankReconciliationNotFound
		}
		return model.BankReconciliation{}, err
	}
	return rec, nil
}
