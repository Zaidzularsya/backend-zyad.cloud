package repository

import (
	"context"
	"time"

	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/platform/database"
)

type FiscalRepository struct {
	db *database.Pool
}

func NewFiscalRepository(db *database.Pool) *FiscalRepository {
	return &FiscalRepository{db: db}
}

// CreateFiscalYear inserts a fiscal year spanning Jan 1 - Dec 31 of the given
// year, plus its 12 monthly periods, in a single transaction.
func (r *FiscalRepository) CreateFiscalYear(ctx context.Context, year int) (model.FiscalYear, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.FiscalYear{}, err
	}
	defer tx.Rollback(ctx)

	startDate := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)

	var fiscalYear model.FiscalYear
	err = tx.QueryRow(ctx, `
		INSERT INTO finance_fiscal_years (year, start_date, end_date)
		VALUES ($1, $2, $3)
		RETURNING id, year, start_date, end_date, status, closed_at, closed_by, created_at, updated_at
	`, year, startDate, endDate).Scan(
		&fiscalYear.ID, &fiscalYear.Year, &fiscalYear.StartDate, &fiscalYear.EndDate,
		&fiscalYear.Status, &fiscalYear.ClosedAt, &fiscalYear.ClosedBy,
		&fiscalYear.CreatedAt, &fiscalYear.UpdatedAt,
	)
	if err != nil {
		return model.FiscalYear{}, err
	}

	for month := 1; month <= 12; month++ {
		periodStart := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		periodEnd := periodStart.AddDate(0, 1, -1)
		if _, err := tx.Exec(ctx, `
			INSERT INTO finance_fiscal_periods (fiscal_year_id, period_number, start_date, end_date)
			VALUES ($1::uuid, $2, $3, $4)
		`, fiscalYear.ID, month, periodStart, periodEnd); err != nil {
			return model.FiscalYear{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.FiscalYear{}, err
	}
	return fiscalYear, nil
}

func (r *FiscalRepository) ListFiscalYears(ctx context.Context) ([]model.FiscalYear, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, year, start_date, end_date, status, closed_at, closed_by, created_at, updated_at
		FROM finance_fiscal_years
		ORDER BY year DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	years := make([]model.FiscalYear, 0)
	for rows.Next() {
		var y model.FiscalYear
		if err := rows.Scan(&y.ID, &y.Year, &y.StartDate, &y.EndDate, &y.Status, &y.ClosedAt, &y.ClosedBy, &y.CreatedAt, &y.UpdatedAt); err != nil {
			return nil, err
		}
		years = append(years, y)
	}
	return years, rows.Err()
}

func (r *FiscalRepository) ListFiscalPeriods(ctx context.Context, fiscalYearID string) ([]model.FiscalPeriod, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, fiscal_year_id, period_number, start_date, end_date, status, closed_at, closed_by, created_at, updated_at
		FROM finance_fiscal_periods
		WHERE fiscal_year_id = $1::uuid
		ORDER BY period_number ASC
	`, fiscalYearID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	periods := make([]model.FiscalPeriod, 0)
	for rows.Next() {
		var p model.FiscalPeriod
		if err := rows.Scan(&p.ID, &p.FiscalYearID, &p.PeriodNumber, &p.StartDate, &p.EndDate, &p.Status, &p.ClosedAt, &p.ClosedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		periods = append(periods, p)
	}
	return periods, rows.Err()
}

// FindPeriodByDate resolves which fiscal period covers the given date.
// Returns pgx.ErrNoRows if no fiscal year/period has been created for it yet.
func (r *FiscalRepository) FindPeriodByDate(ctx context.Context, date time.Time) (model.FiscalPeriod, error) {
	var p model.FiscalPeriod
	err := r.db.QueryRow(ctx, `
		SELECT id, fiscal_year_id, period_number, start_date, end_date, status, closed_at, closed_by, created_at, updated_at
		FROM finance_fiscal_periods
		WHERE start_date <= $1 AND end_date >= $1
	`, date).Scan(&p.ID, &p.FiscalYearID, &p.PeriodNumber, &p.StartDate, &p.EndDate, &p.Status, &p.ClosedAt, &p.ClosedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return model.FiscalPeriod{}, err
	}
	return p, nil
}

func (r *FiscalRepository) FindPeriodByID(ctx context.Context, id string) (model.FiscalPeriod, error) {
	var p model.FiscalPeriod
	err := r.db.QueryRow(ctx, `
		SELECT id, fiscal_year_id, period_number, start_date, end_date, status, closed_at, closed_by, created_at, updated_at
		FROM finance_fiscal_periods
		WHERE id = $1::uuid
	`, id).Scan(&p.ID, &p.FiscalYearID, &p.PeriodNumber, &p.StartDate, &p.EndDate, &p.Status, &p.ClosedAt, &p.ClosedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return model.FiscalPeriod{}, err
	}
	return p, nil
}

func (r *FiscalRepository) SetPeriodStatus(ctx context.Context, id string, status model.FiscalStatus, actorID *string) (model.FiscalPeriod, error) {
	var p model.FiscalPeriod
	var closedAt any
	var closedBy any
	if status == model.FiscalStatusClosed {
		closedAt = time.Now().UTC()
		closedBy = nullableUUID(actorID)
	}
	err := r.db.QueryRow(ctx, `
		UPDATE finance_fiscal_periods
		SET status = $2, closed_at = $3, closed_by = $4::uuid, updated_at = now()
		WHERE id = $1::uuid
		RETURNING id, fiscal_year_id, period_number, start_date, end_date, status, closed_at, closed_by, created_at, updated_at
	`, id, string(status), closedAt, closedBy).Scan(
		&p.ID, &p.FiscalYearID, &p.PeriodNumber, &p.StartDate, &p.EndDate, &p.Status, &p.ClosedAt, &p.ClosedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return model.FiscalPeriod{}, err
	}
	return p, nil
}

func (r *FiscalRepository) SetFiscalYearStatus(ctx context.Context, id string, status model.FiscalStatus, actorID *string) (model.FiscalYear, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.FiscalYear{}, err
	}
	defer tx.Rollback(ctx)

	var closedAt any
	var closedBy any
	if status == model.FiscalStatusClosed {
		closedAt = time.Now().UTC()
		closedBy = nullableUUID(actorID)
	}

	var y model.FiscalYear
	err = tx.QueryRow(ctx, `
		UPDATE finance_fiscal_years
		SET status = $2, closed_at = $3, closed_by = $4::uuid, updated_at = now()
		WHERE id = $1::uuid
		RETURNING id, year, start_date, end_date, status, closed_at, closed_by, created_at, updated_at
	`, id, string(status), closedAt, closedBy).Scan(
		&y.ID, &y.Year, &y.StartDate, &y.EndDate, &y.Status, &y.ClosedAt, &y.ClosedBy, &y.CreatedAt, &y.UpdatedAt,
	)
	if err != nil {
		return model.FiscalYear{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE finance_fiscal_periods
		SET status = $2, closed_at = $3, closed_by = $4::uuid, updated_at = now()
		WHERE fiscal_year_id = $1::uuid
	`, id, string(status), closedAt, closedBy); err != nil {
		return model.FiscalYear{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.FiscalYear{}, err
	}
	return y, nil
}
