package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/platform/database"
)

type TaxRepository struct {
	db *database.Pool
}

func NewTaxRepository(db *database.Pool) *TaxRepository {
	return &TaxRepository{db: db}
}

func (r *TaxRepository) ListTypes(ctx context.Context) ([]model.TaxType, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, code, name, category, created_at, updated_at
		FROM finance_tax_types
		ORDER BY code ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	types := make([]model.TaxType, 0)
	for rows.Next() {
		var t model.TaxType
		if err := rows.Scan(&t.ID, &t.Code, &t.Name, &t.Category, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	return types, rows.Err()
}

type CreateTaxRateParams struct {
	TaxTypeID     string
	RatePercent   string
	EffectiveDate time.Time
	EndDate       *time.Time
	Notes         string
}

func (r *TaxRepository) CreateRate(ctx context.Context, params CreateTaxRateParams) (model.TaxRate, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO finance_tax_rates (tax_type_id, rate_percent, effective_date, end_date, notes)
		VALUES ($1::uuid, $2::numeric, $3, $4, NULLIF($5, ''))
		RETURNING id
	`, strings.TrimSpace(params.TaxTypeID), amountOrZero(params.RatePercent), params.EffectiveDate, params.EndDate, params.Notes).
		Scan(&id)
	if err != nil {
		return model.TaxRate{}, err
	}
	return r.FindRateByID(ctx, id)
}

func (r *TaxRepository) FindRateByID(ctx context.Context, id string) (model.TaxRate, error) {
	var rate model.TaxRate
	err := r.db.QueryRow(ctx, `
		SELECT tr.id, tr.tax_type_id, tt.code, tr.rate_percent::text, tr.effective_date, tr.end_date,
			COALESCE(tr.notes, ''), tr.created_at, tr.updated_at
		FROM finance_tax_rates tr
		JOIN finance_tax_types tt ON tt.id = tr.tax_type_id
		WHERE tr.id = $1::uuid
	`, strings.TrimSpace(id)).Scan(
		&rate.ID, &rate.TaxTypeID, &rate.TaxTypeCode, &rate.RatePercent, &rate.EffectiveDate, &rate.EndDate,
		&rate.Notes, &rate.CreatedAt, &rate.UpdatedAt,
	)
	if err != nil {
		return model.TaxRate{}, err
	}
	return rate, nil
}

func (r *TaxRepository) ListRates(ctx context.Context, taxTypeID string) ([]model.TaxRate, error) {
	rows, err := r.db.Query(ctx, `
		SELECT tr.id, tr.tax_type_id, tt.code, tr.rate_percent::text, tr.effective_date, tr.end_date,
			COALESCE(tr.notes, ''), tr.created_at, tr.updated_at
		FROM finance_tax_rates tr
		JOIN finance_tax_types tt ON tt.id = tr.tax_type_id
		WHERE tr.tax_type_id = $1::uuid
		ORDER BY tr.effective_date DESC
	`, strings.TrimSpace(taxTypeID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rates := make([]model.TaxRate, 0)
	for rows.Next() {
		var rate model.TaxRate
		if err := rows.Scan(&rate.ID, &rate.TaxTypeID, &rate.TaxTypeCode, &rate.RatePercent, &rate.EffectiveDate,
			&rate.EndDate, &rate.Notes, &rate.CreatedAt, &rate.UpdatedAt); err != nil {
			return nil, err
		}
		rates = append(rates, rate)
	}
	return rates, rows.Err()
}

// taxAccountIsDebited reports whether tax_account should be debited (true)
// or credited (false) for the given direction, based on the account's own
// normal_balance: 'increase' always moves the account further in its normal
// direction, 'decrease' always moves it the other way. This is what makes
// the same 'increase'/'decrease' vocabulary correct for both a
// liability-normal tax account (e.g. PPN Keluaran payable, credit-normal)
// and an asset-normal one (e.g. PPN Masukan recoverable, debit-normal).
func taxAccountIsDebited(direction model.TaxDirection, normalBalance string) bool {
	return (direction == model.TaxDirectionIncrease) == (normalBalance == "debit")
}

type CreateTaxTransactionParams struct {
	TaxTypeID       string
	TransactionDate time.Time
	ReferenceNumber string
	Amount          string
	Direction       model.TaxDirection
	TaxAccountID    string
	ContraAccountID string
	Description     string
	EntryNumber     string
	CreatedBy       *string
}

// CreateTransaction posts the tax journal entry and inserts the
// tax_transactions row referencing it, in one DB transaction. Which side of
// tax_account gets debited is derived from that account's own normal_balance
// (already set by the chart of accounts), not hardcoded per tax type — an
// 'increase' always moves the account further in its normal direction, a
// 'decrease' always moves it the other way, which is correct regardless of
// whether tax_account is a liability (e.g. PPN Keluaran payable) or an asset
// (e.g. PPN Masukan recoverable).
func (r *TaxRepository) CreateTransaction(ctx context.Context, params CreateTaxTransactionParams) (model.TaxTransaction, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.TaxTransaction{}, err
	}
	defer tx.Rollback(ctx)

	var normalBalance string
	if err := tx.QueryRow(ctx, `SELECT normal_balance FROM finance_accounts WHERE id = $1::uuid`, params.TaxAccountID).
		Scan(&normalBalance); err != nil {
		return model.TaxTransaction{}, err
	}

	var lines []CreateJournalLineParams
	if taxAccountIsDebited(params.Direction, normalBalance) {
		lines = []CreateJournalLineParams{
			{AccountID: params.TaxAccountID, Debit: params.Amount, SubledgerType: model.SubledgerTaxCode, SubledgerID: &params.TaxTypeID},
			{AccountID: params.ContraAccountID, Credit: params.Amount},
		}
	} else {
		lines = []CreateJournalLineParams{
			{AccountID: params.ContraAccountID, Debit: params.Amount},
			{AccountID: params.TaxAccountID, Credit: params.Amount, SubledgerType: model.SubledgerTaxCode, SubledgerID: &params.TaxTypeID},
		}
	}

	entryID, err := InsertJournalEntryTx(ctx, tx, CreateJournalEntryParams{
		EntryNumber: params.EntryNumber,
		EntryDate:   params.TransactionDate,
		SourceType:  model.JournalSourceTax,
		Reference:   params.ReferenceNumber,
		Description: params.Description,
		CreatedBy:   params.CreatedBy,
		Lines:       lines,
	})
	if err != nil {
		return model.TaxTransaction{}, err
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO finance_tax_transactions (
			tax_type_id, transaction_date, reference_number, amount, direction,
			tax_account_id, contra_account_id, description, journal_entry_id, created_by
		)
		VALUES ($1::uuid, $2, NULLIF($3, ''), $4::numeric, $5, $6::uuid, $7::uuid, NULLIF($8, ''), $9::uuid, $10::uuid)
		RETURNING id
	`,
		params.TaxTypeID, params.TransactionDate, params.ReferenceNumber, amountOrZero(params.Amount), string(params.Direction),
		params.TaxAccountID, params.ContraAccountID, params.Description, entryID, nullableUUID(params.CreatedBy),
	).Scan(&id)
	if err != nil {
		return model.TaxTransaction{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.TaxTransaction{}, err
	}
	return r.FindTransactionByID(ctx, id)
}

const taxTransactionSelectColumns = `
	t.id,
	t.tax_type_id,
	tt.code,
	tt.name,
	t.transaction_date,
	COALESCE(t.reference_number, ''),
	t.amount::text,
	t.direction,
	t.tax_account_id,
	ta.account_code,
	ta.account_name,
	t.contra_account_id,
	ca.account_code,
	ca.account_name,
	COALESCE(t.description, ''),
	t.journal_entry_id,
	t.created_at,
	t.updated_at
`

const taxTransactionFrom = `
	FROM finance_tax_transactions t
	JOIN finance_tax_types tt ON tt.id = t.tax_type_id
	JOIN finance_accounts ta ON ta.id = t.tax_account_id
	JOIN finance_accounts ca ON ca.id = t.contra_account_id
`

func (r *TaxRepository) FindTransactionByID(ctx context.Context, id string) (model.TaxTransaction, error) {
	var t model.TaxTransaction
	err := r.db.QueryRow(ctx, `
		SELECT `+taxTransactionSelectColumns+taxTransactionFrom+`
		WHERE t.id = $1::uuid
	`, strings.TrimSpace(id)).Scan(taxTransactionScanDest(&t)...)
	if err != nil {
		return model.TaxTransaction{}, err
	}
	return t, nil
}

type TaxTransactionListFilter struct {
	TaxTypeID string
	StartDate *time.Time
	EndDate   *time.Time
	Limit     int
	Offset    int
}

func (r *TaxRepository) ListTransactions(ctx context.Context, filter TaxTransactionListFilter) ([]model.TaxTransaction, int64, error) {
	conditions := []string{}
	args := []any{}
	if filter.TaxTypeID != "" {
		args = append(args, filter.TaxTypeID)
		conditions = append(conditions, fmt.Sprintf("t.tax_type_id = $%d::uuid", len(args)))
	}
	if filter.StartDate != nil {
		args = append(args, *filter.StartDate)
		conditions = append(conditions, fmt.Sprintf("t.transaction_date >= $%d", len(args)))
	}
	if filter.EndDate != nil {
		args = append(args, *filter.EndDate)
		conditions = append(conditions, fmt.Sprintf("t.transaction_date <= $%d", len(args)))
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) "+taxTransactionFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := pagination(filter.Limit, filter.Offset)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+taxTransactionSelectColumns+taxTransactionFrom+where+`
		ORDER BY t.transaction_date DESC, t.created_at DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	transactions := make([]model.TaxTransaction, 0)
	for rows.Next() {
		var t model.TaxTransaction
		if err := rows.Scan(taxTransactionScanDest(&t)...); err != nil {
			return nil, 0, err
		}
		transactions = append(transactions, t)
	}
	return transactions, total, rows.Err()
}

func taxTransactionScanDest(t *model.TaxTransaction) []any {
	return []any{
		&t.ID, &t.TaxTypeID, &t.TaxTypeCode, &t.TaxTypeName, &t.TransactionDate, &t.ReferenceNumber,
		&t.Amount, &t.Direction, &t.TaxAccountID, &t.TaxAccountCode, &t.TaxAccountName,
		&t.ContraAccountID, &t.ContraAccountCode, &t.ContraAccountName, &t.Description,
		&t.JournalEntryID, &t.CreatedAt, &t.UpdatedAt,
	}
}

// Summary aggregates tax_transactions (not a stored summary table) by tax
// type over [start, end], splitting increase vs decrease totals.
func (r *TaxRepository) Summary(ctx context.Context, start, end time.Time) ([]model.TaxSummaryRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT tt.id, tt.code, tt.name,
			COALESCE(SUM(CASE WHEN t.direction = 'increase' THEN t.amount ELSE 0 END), 0)::text,
			COALESCE(SUM(CASE WHEN t.direction = 'decrease' THEN t.amount ELSE 0 END), 0)::text,
			COALESCE(SUM(CASE WHEN t.direction = 'increase' THEN t.amount ELSE -t.amount END), 0)::text
		FROM finance_tax_types tt
		LEFT JOIN finance_tax_transactions t
			ON t.tax_type_id = tt.id AND t.transaction_date BETWEEN $1 AND $2
		GROUP BY tt.id, tt.code, tt.name
		ORDER BY tt.code ASC
	`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary := make([]model.TaxSummaryRow, 0)
	for rows.Next() {
		var row model.TaxSummaryRow
		if err := rows.Scan(&row.TaxTypeID, &row.TaxTypeCode, &row.TaxTypeName, &row.Increase, &row.Decrease, &row.Net); err != nil {
			return nil, err
		}
		summary = append(summary, row)
	}
	return summary, rows.Err()
}
