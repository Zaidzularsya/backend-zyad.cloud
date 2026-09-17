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

// ErrFiscalPeriodClosed is returned by Create/Reverse when the resolved
// fiscal period for the entry date is closed for posting.
var ErrFiscalPeriodClosed = errors.New("finance: fiscal period is closed")

// ErrNoFiscalPeriod is returned by Create/Reverse when no fiscal period has
// been created yet to cover the given date.
var ErrNoFiscalPeriod = errors.New("finance: no fiscal period covers this date")

// ErrJournalEntryNotPosted is returned by Reverse when the original entry is
// not currently in posted status.
var ErrJournalEntryNotPosted = errors.New("finance: journal entry is not posted")

const journalEntrySelectColumns = `
	je.id,
	je.entry_number,
	je.entry_date,
	je.fiscal_period_id,
	je.source_type,
	je.source_id,
	COALESCE(je.reference, ''),
	COALESCE(je.description, ''),
	je.status,
	je.posted_at,
	je.reversed_by_entry_id,
	je.created_by,
	je.created_at,
	je.updated_at
`

const journalLineSelectColumns = `
	jl.id,
	jl.journal_entry_id,
	jl.line_number,
	jl.account_id,
	a.account_code,
	a.account_name,
	jl.debit::text,
	jl.credit::text,
	COALESCE(jl.description, ''),
	jl.subledger_type,
	jl.subledger_id,
	jl.created_at
`

type JournalRepository struct {
	db *database.Pool
}

func NewJournalRepository(db *database.Pool) *JournalRepository {
	return &JournalRepository{db: db}
}

type CreateJournalLineParams struct {
	AccountID     string
	Debit         string
	Credit        string
	Description   string
	SubledgerType model.SubledgerType
	SubledgerID   *string
}

type CreateJournalEntryParams struct {
	EntryNumber string
	EntryDate   time.Time
	SourceType  model.JournalSourceType
	SourceID    *string
	Reference   string
	Description string
	CreatedBy   *string
	Lines       []CreateJournalLineParams
}

type JournalEntryListFilter struct {
	StartDate *time.Time
	EndDate   *time.Time
	Status    model.JournalStatus
	Limit     int
	Offset    int
}

// Create resolves the fiscal period for EntryDate, verifies it is open, and
// inserts the journal entry with status=posted plus all of its lines in one
// transaction. Callers must have already validated that the lines balance
// (sum(debit) == sum(credit)) — this is a service-layer invariant, not
// re-checked here.
func (r *JournalRepository) Create(ctx context.Context, params CreateJournalEntryParams) (model.JournalEntry, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.JournalEntry{}, err
	}
	defer tx.Rollback(ctx)

	var fiscalPeriodID string
	var periodStatus string
	err = tx.QueryRow(ctx, `
		SELECT id, status
		FROM finance_fiscal_periods
		WHERE start_date <= $1 AND end_date >= $1
		FOR UPDATE
	`, params.EntryDate).Scan(&fiscalPeriodID, &periodStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.JournalEntry{}, ErrNoFiscalPeriod
		}
		return model.JournalEntry{}, err
	}
	if periodStatus == string(model.FiscalStatusClosed) {
		return model.JournalEntry{}, ErrFiscalPeriodClosed
	}

	now := time.Now().UTC()
	var entryID string
	err = tx.QueryRow(ctx, `
		INSERT INTO finance_journal_entries (
			entry_number, entry_date, fiscal_period_id, source_type, source_id,
			reference, description, status, posted_at, posted_by, created_by
		)
		VALUES ($1, $2, $3::uuid, $4, $5::uuid, NULLIF($6, ''), NULLIF($7, ''), 'posted', $8, $9::uuid, $9::uuid)
		RETURNING id
	`,
		params.EntryNumber, params.EntryDate, fiscalPeriodID, string(params.SourceType),
		nullableUUID(params.SourceID), params.Reference, params.Description, now, nullableUUID(params.CreatedBy),
	).Scan(&entryID)
	if err != nil {
		return model.JournalEntry{}, err
	}

	for i, line := range params.Lines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO finance_journal_lines (
				journal_entry_id, line_number, account_id, debit, credit, description, subledger_type, subledger_id
			)
			VALUES ($1::uuid, $2, $3::uuid, $4::numeric, $5::numeric, NULLIF($6, ''), $7, $8::uuid)
		`,
			entryID, i+1, line.AccountID, amountOrZero(line.Debit), amountOrZero(line.Credit),
			line.Description, subledgerTypeOrDefault(line.SubledgerType), nullableUUID(line.SubledgerID),
		); err != nil {
			return model.JournalEntry{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.JournalEntry{}, err
	}
	return r.FindByID(ctx, entryID)
}

func (r *JournalRepository) FindByID(ctx context.Context, id string) (model.JournalEntry, error) {
	var entry model.JournalEntry
	err := r.db.QueryRow(ctx, `
		SELECT `+journalEntrySelectColumns+`
		FROM finance_journal_entries je
		WHERE je.id = $1::uuid
	`, strings.TrimSpace(id)).Scan(journalEntryScanDest(&entry)...)
	if err != nil {
		return model.JournalEntry{}, err
	}

	lines, err := r.listLines(ctx, entry.ID)
	if err != nil {
		return model.JournalEntry{}, err
	}
	entry.Lines = lines
	return entry, nil
}

func (r *JournalRepository) listLines(ctx context.Context, journalEntryID string) ([]model.JournalLine, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+journalLineSelectColumns+`
		FROM finance_journal_lines jl
		JOIN finance_accounts a ON a.id = jl.account_id
		WHERE jl.journal_entry_id = $1::uuid
		ORDER BY jl.line_number ASC
	`, journalEntryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := make([]model.JournalLine, 0)
	for rows.Next() {
		var line model.JournalLine
		if err := rows.Scan(journalLineScanDest(&line)...); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func (r *JournalRepository) List(ctx context.Context, filter JournalEntryListFilter) ([]model.JournalEntry, int64, error) {
	where, args := journalWhere(filter)

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM finance_journal_entries je"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := pagination(filter.Limit, filter.Offset)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+journalEntrySelectColumns+`
		FROM finance_journal_entries je`+where+`
		ORDER BY je.entry_date DESC, je.entry_number DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries := make([]model.JournalEntry, 0)
	for rows.Next() {
		var entry model.JournalEntry
		if err := rows.Scan(journalEntryScanDest(&entry)...); err != nil {
			return nil, 0, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	for i := range entries {
		lines, err := r.listLines(ctx, entries[i].ID)
		if err != nil {
			return nil, 0, err
		}
		entries[i].Lines = lines
	}
	return entries, total, nil
}

func (r *JournalRepository) NextEntryNumber(ctx context.Context, year int) (string, error) {
	var seq int64
	if err := r.db.QueryRow(ctx, `SELECT nextval('finance_journal_entry_number_seq')`).Scan(&seq); err != nil {
		return "", err
	}
	return fmt.Sprintf("JE-%d-%06d", year, seq), nil
}

// Reverse creates a mirrored journal entry (debit/credit swapped) dated
// reversalDate, marks the original entry as reversed, and links the two.
// Both the original entry's period (implicitly, since it must already be
// posted) and the reversal date's period must be open.
func (r *JournalRepository) Reverse(ctx context.Context, originalID string, reversalDate time.Time, entryNumber string, reason string, actorID *string) (model.JournalEntry, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.JournalEntry{}, err
	}
	defer tx.Rollback(ctx)

	var status string
	var sourceType string
	var originalNumber string
	err = tx.QueryRow(ctx, `
		SELECT status, source_type, entry_number
		FROM finance_journal_entries
		WHERE id = $1::uuid
		FOR UPDATE
	`, originalID).Scan(&status, &sourceType, &originalNumber)
	if err != nil {
		return model.JournalEntry{}, err
	}
	if status != string(model.JournalStatusPosted) {
		return model.JournalEntry{}, ErrJournalEntryNotPosted
	}

	var fiscalPeriodID string
	var periodStatus string
	err = tx.QueryRow(ctx, `
		SELECT id, status FROM finance_fiscal_periods
		WHERE start_date <= $1 AND end_date >= $1
		FOR UPDATE
	`, reversalDate).Scan(&fiscalPeriodID, &periodStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.JournalEntry{}, ErrNoFiscalPeriod
		}
		return model.JournalEntry{}, err
	}
	if periodStatus == string(model.FiscalStatusClosed) {
		return model.JournalEntry{}, ErrFiscalPeriodClosed
	}

	now := time.Now().UTC()
	description := "Reversal of " + originalNumber
	if strings.TrimSpace(reason) != "" {
		description += ": " + reason
	}

	var newEntryID string
	err = tx.QueryRow(ctx, `
		INSERT INTO finance_journal_entries (
			entry_number, entry_date, fiscal_period_id, source_type,
			reference, description, status, posted_at, posted_by, created_by
		)
		VALUES ($1, $2, $3::uuid, $4, $5, $6, 'posted', $7, $8::uuid, $8::uuid)
		RETURNING id
	`, entryNumber, reversalDate, fiscalPeriodID, sourceType, originalNumber, description, now, nullableUUID(actorID)).
		Scan(&newEntryID)
	if err != nil {
		return model.JournalEntry{}, err
	}

	rows, err := tx.Query(ctx, `
		SELECT account_id, debit, credit, COALESCE(description, ''), subledger_type, subledger_id, line_number
		FROM finance_journal_lines
		WHERE journal_entry_id = $1::uuid
		ORDER BY line_number ASC
	`, originalID)
	if err != nil {
		return model.JournalEntry{}, err
	}
	type originalLine struct {
		accountID     string
		debit         string
		credit        string
		description   string
		subledgerType string
		subledgerID   *string
		lineNumber    int
	}
	originalLines := make([]originalLine, 0)
	for rows.Next() {
		var l originalLine
		if err := rows.Scan(&l.accountID, &l.debit, &l.credit, &l.description, &l.subledgerType, &l.subledgerID, &l.lineNumber); err != nil {
			rows.Close()
			return model.JournalEntry{}, err
		}
		originalLines = append(originalLines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return model.JournalEntry{}, err
	}

	for _, l := range originalLines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO finance_journal_lines (
				journal_entry_id, line_number, account_id, debit, credit, description, subledger_type, subledger_id
			)
			VALUES ($1::uuid, $2, $3::uuid, $4::numeric, $5::numeric, $6, $7, $8::uuid)
		`, newEntryID, l.lineNumber, l.accountID, l.credit, l.debit, l.description, l.subledgerType, l.subledgerID); err != nil {
			return model.JournalEntry{}, err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE finance_journal_entries
		SET status = 'reversed', reversed_by_entry_id = $2::uuid, updated_at = now()
		WHERE id = $1::uuid
	`, originalID, newEntryID); err != nil {
		return model.JournalEntry{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.JournalEntry{}, err
	}
	return r.FindByID(ctx, newEntryID)
}

func journalWhere(filter JournalEntryListFilter) (string, []any) {
	conditions := []string{}
	args := []any{}
	if filter.StartDate != nil {
		args = append(args, *filter.StartDate)
		conditions = append(conditions, fmt.Sprintf("je.entry_date >= $%d", len(args)))
	}
	if filter.EndDate != nil {
		args = append(args, *filter.EndDate)
		conditions = append(conditions, fmt.Sprintf("je.entry_date <= $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		conditions = append(conditions, fmt.Sprintf("je.status = $%d", len(args)))
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func journalEntryScanDest(entry *model.JournalEntry) []any {
	return []any{
		&entry.ID,
		&entry.EntryNumber,
		&entry.EntryDate,
		&entry.FiscalPeriodID,
		&entry.SourceType,
		&entry.SourceID,
		&entry.Reference,
		&entry.Description,
		&entry.Status,
		&entry.PostedAt,
		&entry.ReversedByEntryID,
		&entry.CreatedBy,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	}
}

func journalLineScanDest(line *model.JournalLine) []any {
	return []any{
		&line.ID,
		&line.JournalEntryID,
		&line.LineNumber,
		&line.AccountID,
		&line.AccountCode,
		&line.AccountName,
		&line.Debit,
		&line.Credit,
		&line.Description,
		&line.SubledgerType,
		&line.SubledgerID,
		&line.CreatedAt,
	}
}

func subledgerTypeOrDefault(value model.SubledgerType) string {
	if value == "" {
		return string(model.SubledgerNone)
	}
	return string(value)
}
