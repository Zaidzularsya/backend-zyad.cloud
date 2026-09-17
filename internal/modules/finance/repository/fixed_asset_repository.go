package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"zyad.cloud/internal/modules/finance/model"
	"zyad.cloud/internal/platform/database"
)

type FixedAssetRepository struct {
	db *database.Pool
}

func NewFixedAssetRepository(db *database.Pool) *FixedAssetRepository {
	return &FixedAssetRepository{db: db}
}

const assetCategorySelectColumns = `
	ac.id,
	ac.code,
	ac.name,
	ac.asset_account_id,
	aa.account_code,
	aa.account_name,
	ac.accumulated_depreciation_account_id,
	ada.account_code,
	ada.account_name,
	ac.depreciation_expense_account_id,
	dea.account_code,
	dea.account_name,
	ac.default_useful_life_months,
	ac.is_active,
	ac.created_at,
	ac.updated_at
`

const assetCategoryFrom = `
	FROM finance_asset_categories ac
	JOIN finance_accounts aa ON aa.id = ac.asset_account_id
	JOIN finance_accounts ada ON ada.id = ac.accumulated_depreciation_account_id
	JOIN finance_accounts dea ON dea.id = ac.depreciation_expense_account_id
`

type CreateAssetCategoryParams struct {
	Code                             string
	Name                             string
	AssetAccountID                   string
	AccumulatedDepreciationAccountID string
	DepreciationExpenseAccountID     string
	DefaultUsefulLifeMonths          *int
}

func (r *FixedAssetRepository) CreateCategory(ctx context.Context, params CreateAssetCategoryParams) (model.AssetCategory, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO finance_asset_categories (
			code, name, asset_account_id, accumulated_depreciation_account_id,
			depreciation_expense_account_id, default_useful_life_months
		)
		VALUES ($1, $2, $3::uuid, $4::uuid, $5::uuid, $6)
		RETURNING id
	`, strings.TrimSpace(params.Code), strings.TrimSpace(params.Name),
		strings.TrimSpace(params.AssetAccountID), strings.TrimSpace(params.AccumulatedDepreciationAccountID),
		strings.TrimSpace(params.DepreciationExpenseAccountID), params.DefaultUsefulLifeMonths,
	).Scan(&id)
	if err != nil {
		return model.AssetCategory{}, err
	}
	return r.FindCategoryByID(ctx, id)
}

func (r *FixedAssetRepository) FindCategoryByID(ctx context.Context, id string) (model.AssetCategory, error) {
	var c model.AssetCategory
	err := r.db.QueryRow(ctx, `
		SELECT `+assetCategorySelectColumns+assetCategoryFrom+`
		WHERE ac.id = $1::uuid
	`, strings.TrimSpace(id)).Scan(assetCategoryScanDest(&c)...)
	if err != nil {
		return model.AssetCategory{}, err
	}
	return c, nil
}

type AssetCategoryListFilter struct {
	IncludeInactive bool
}

func (r *FixedAssetRepository) ListCategories(ctx context.Context, filter AssetCategoryListFilter) ([]model.AssetCategory, error) {
	where := ""
	if !filter.IncludeInactive {
		where = " WHERE ac.is_active = true"
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+assetCategorySelectColumns+assetCategoryFrom+where+`
		ORDER BY ac.code ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := make([]model.AssetCategory, 0)
	for rows.Next() {
		var c model.AssetCategory
		if err := rows.Scan(assetCategoryScanDest(&c)...); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, rows.Err()
}

func assetCategoryScanDest(c *model.AssetCategory) []any {
	return []any{
		&c.ID, &c.Code, &c.Name,
		&c.AssetAccountID, &c.AssetAccountCode, &c.AssetAccountName,
		&c.AccumulatedDepreciationAccountID, &c.AccumulatedDepreciationAccountCode, &c.AccumulatedDepreciationAccountName,
		&c.DepreciationExpenseAccountID, &c.DepreciationExpenseAccountCode, &c.DepreciationExpenseAccountName,
		&c.DefaultUsefulLifeMonths, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
	}
}

const fixedAssetSelectColumns = `
	fa.id,
	fa.asset_category_id,
	ac.code,
	ac.name,
	fa.asset_code,
	fa.asset_name,
	fa.acquisition_date,
	fa.acquisition_cost::text,
	fa.salvage_value::text,
	fa.useful_life_months,
	COALESCE(fa.description, ''),
	fa.contra_account_id,
	fa.acquisition_journal_entry_id,
	fa.status,
	fa.created_at,
	fa.updated_at,
	COALESCE(ds.accumulated, 0)::text,
	(fa.acquisition_cost - COALESCE(ds.accumulated, 0))::text
`

const fixedAssetFrom = `
	FROM finance_fixed_assets fa
	JOIN finance_asset_categories ac ON ac.id = fa.asset_category_id
	LEFT JOIN (
		SELECT fixed_asset_id, SUM(depreciation_amount) AS accumulated
		FROM finance_depreciation_schedules
		WHERE status = 'posted'
		GROUP BY fixed_asset_id
	) ds ON ds.fixed_asset_id = fa.id
`

type CreateFixedAssetParams struct {
	AssetCategoryID  string
	AssetCode        string
	AssetName        string
	AcquisitionDate  time.Time
	AcquisitionCost  string
	SalvageValue     string
	UsefulLifeMonths int
	Description      string
	ContraAccountID  string
	EntryNumber      string
	CreatedBy        *string
}

// ScheduleAmount is one period's depreciation amount, computed by the service
// layer using exact decimal (big.Rat) arithmetic so the schedule always sums
// to exactly acquisition_cost - salvage_value.
type ScheduleAmount struct {
	SequenceNumber int
	PeriodDate     time.Time
	Amount         string
}

// CreateAsset posts the acquisition journal entry (Dr asset account / Cr the
// caller-chosen contra account), inserts the fixed_assets row referencing it,
// and inserts the full depreciation schedule (all rows 'pending') — all in
// one DB transaction.
func (r *FixedAssetRepository) CreateAsset(ctx context.Context, params CreateFixedAssetParams, schedule []ScheduleAmount) (model.FixedAsset, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.FixedAsset{}, err
	}
	defer tx.Rollback(ctx)

	var assetAccountID string
	if err := tx.QueryRow(ctx, `SELECT asset_account_id FROM finance_asset_categories WHERE id = $1::uuid`, params.AssetCategoryID).
		Scan(&assetAccountID); err != nil {
		return model.FixedAsset{}, err
	}

	entryID, err := InsertJournalEntryTx(ctx, tx, CreateJournalEntryParams{
		EntryNumber: params.EntryNumber,
		EntryDate:   params.AcquisitionDate,
		SourceType:  model.JournalSourceManual,
		Description: fmt.Sprintf("Perolehan aset tetap %s (%s)", params.AssetCode, params.AssetName),
		CreatedBy:   params.CreatedBy,
		Lines: []CreateJournalLineParams{
			{AccountID: assetAccountID, Debit: params.AcquisitionCost},
			{AccountID: params.ContraAccountID, Credit: params.AcquisitionCost},
		},
	})
	if err != nil {
		return model.FixedAsset{}, err
	}

	var assetID string
	err = tx.QueryRow(ctx, `
		INSERT INTO finance_fixed_assets (
			asset_category_id, asset_code, asset_name, acquisition_date, acquisition_cost,
			salvage_value, useful_life_months, description, contra_account_id,
			acquisition_journal_entry_id, created_by
		)
		VALUES ($1::uuid, $2, $3, $4, $5::numeric, $6::numeric, $7, NULLIF($8, ''), $9::uuid, $10::uuid, $11::uuid)
		RETURNING id
	`,
		params.AssetCategoryID, strings.TrimSpace(params.AssetCode), strings.TrimSpace(params.AssetName),
		params.AcquisitionDate, amountOrZero(params.AcquisitionCost), amountOrZero(params.SalvageValue),
		params.UsefulLifeMonths, params.Description, params.ContraAccountID, entryID, nullableUUID(params.CreatedBy),
	).Scan(&assetID)
	if err != nil {
		return model.FixedAsset{}, err
	}

	for _, s := range schedule {
		if _, err := tx.Exec(ctx, `
			INSERT INTO finance_depreciation_schedules (fixed_asset_id, sequence_number, period_date, depreciation_amount)
			VALUES ($1::uuid, $2, $3, $4::numeric)
		`, assetID, s.SequenceNumber, s.PeriodDate, amountOrZero(s.Amount)); err != nil {
			return model.FixedAsset{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.FixedAsset{}, err
	}
	return r.FindAssetByID(ctx, assetID)
}

func (r *FixedAssetRepository) FindAssetByID(ctx context.Context, id string) (model.FixedAsset, error) {
	var a model.FixedAsset
	err := r.db.QueryRow(ctx, `
		SELECT `+fixedAssetSelectColumns+fixedAssetFrom+`
		WHERE fa.id = $1::uuid
	`, strings.TrimSpace(id)).Scan(fixedAssetScanDest(&a)...)
	if err != nil {
		return model.FixedAsset{}, err
	}
	return a, nil
}

type FixedAssetListFilter struct {
	AssetCategoryID string
	Status          model.AssetStatus
	Limit           int
	Offset          int
}

func (r *FixedAssetRepository) ListAssets(ctx context.Context, filter FixedAssetListFilter) ([]model.FixedAsset, int64, error) {
	conditions := []string{}
	args := []any{}
	if filter.AssetCategoryID != "" {
		args = append(args, filter.AssetCategoryID)
		conditions = append(conditions, fmt.Sprintf("fa.asset_category_id = $%d::uuid", len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		conditions = append(conditions, fmt.Sprintf("fa.status = $%d", len(args)))
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) "+fixedAssetFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := pagination(filter.Limit, filter.Offset)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+fixedAssetSelectColumns+fixedAssetFrom+where+`
		ORDER BY fa.asset_code ASC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	assets := make([]model.FixedAsset, 0)
	for rows.Next() {
		var a model.FixedAsset
		if err := rows.Scan(fixedAssetScanDest(&a)...); err != nil {
			return nil, 0, err
		}
		assets = append(assets, a)
	}
	return assets, total, rows.Err()
}

func fixedAssetScanDest(a *model.FixedAsset) []any {
	return []any{
		&a.ID, &a.AssetCategoryID, &a.AssetCategoryCode, &a.AssetCategoryName,
		&a.AssetCode, &a.AssetName, &a.AcquisitionDate, &a.AcquisitionCost, &a.SalvageValue,
		&a.UsefulLifeMonths, &a.Description, &a.ContraAccountID, &a.AcquisitionJournalEntryID,
		&a.Status, &a.CreatedAt, &a.UpdatedAt, &a.AccumulatedDepreciation, &a.BookValue,
	}
}

const depreciationScheduleSelectColumns = `
	ds.id,
	ds.fixed_asset_id,
	fa.asset_code,
	fa.asset_name,
	ds.sequence_number,
	ds.period_date,
	ds.depreciation_amount::text,
	ds.status,
	ds.journal_entry_id,
	ds.posted_at,
	ds.created_at
`

const depreciationScheduleFrom = `
	FROM finance_depreciation_schedules ds
	JOIN finance_fixed_assets fa ON fa.id = ds.fixed_asset_id
`

func (r *FixedAssetRepository) ListSchedules(ctx context.Context, fixedAssetID string) ([]model.DepreciationSchedule, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+depreciationScheduleSelectColumns+depreciationScheduleFrom+`
		WHERE ds.fixed_asset_id = $1::uuid
		ORDER BY ds.sequence_number ASC
	`, strings.TrimSpace(fixedAssetID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schedules := make([]model.DepreciationSchedule, 0)
	for rows.Next() {
		var s model.DepreciationSchedule
		if err := rows.Scan(depreciationScheduleScanDest(&s)...); err != nil {
			return nil, err
		}
		schedules = append(schedules, s)
	}
	return schedules, rows.Err()
}

func depreciationScheduleScanDest(s *model.DepreciationSchedule) []any {
	return []any{
		&s.ID, &s.FixedAssetID, &s.AssetCode, &s.AssetName, &s.SequenceNumber,
		&s.PeriodDate, &s.DepreciationAmount, &s.Status, &s.JournalEntryID, &s.PostedAt, &s.CreatedAt,
	}
}

// PostDepreciationThroughDate posts a journal entry (Dr depreciation expense
// / Cr accumulated depreciation, subledger asset=fixed_asset_id) for every
// 'pending' schedule row with period_date <= asOf, across all assets. Each
// row is posted in its own DB transaction, so one row failing (typically
// because its fiscal period hasn't been created yet) does not block the
// others — callers get a per-row result and can retry failures later once
// the missing period exists. Re-running with the same or an earlier asOf is
// a no-op for rows already posted (they are filtered out by status).
func (r *FixedAssetRepository) PostDepreciationThroughDate(ctx context.Context, asOf time.Time, entryNumbers func(year int) (string, error), actorID *string) ([]DepreciationPostResult, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ds.id, ds.fixed_asset_id, fa.asset_code, ds.period_date, ds.depreciation_amount::text,
			ac.depreciation_expense_account_id, ac.accumulated_depreciation_account_id
		FROM finance_depreciation_schedules ds
		JOIN finance_fixed_assets fa ON fa.id = ds.fixed_asset_id
		JOIN finance_asset_categories ac ON ac.id = fa.asset_category_id
		WHERE ds.status = 'pending' AND ds.period_date <= $1
		ORDER BY ds.period_date ASC, fa.asset_code ASC
	`, asOf)
	if err != nil {
		return nil, err
	}
	type pendingRow struct {
		scheduleID       string
		fixedAssetID     string
		assetCode        string
		periodDate       time.Time
		amount           string
		expenseAccountID string
		accumAccountID   string
	}
	pending := make([]pendingRow, 0)
	for rows.Next() {
		var p pendingRow
		if err := rows.Scan(&p.scheduleID, &p.fixedAssetID, &p.assetCode, &p.periodDate, &p.amount,
			&p.expenseAccountID, &p.accumAccountID); err != nil {
			rows.Close()
			return nil, err
		}
		pending = append(pending, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	results := make([]DepreciationPostResult, 0, len(pending))
	for _, p := range pending {
		result := DepreciationPostResult{ScheduleID: p.scheduleID, AssetCode: p.assetCode, PeriodDate: p.periodDate}
		if err := r.postOneDepreciationRow(ctx, p.scheduleID, p.fixedAssetID, p.periodDate, p.amount, p.expenseAccountID, p.accumAccountID, entryNumbers, actorID); err != nil {
			result.Status = "failed"
			result.Reason = depreciationFailureReason(err)
		} else {
			result.Status = "posted"
		}
		results = append(results, result)
	}
	return results, nil
}

func depreciationFailureReason(err error) string {
	if errors.Is(err, ErrNoFiscalPeriod) {
		return "fiscal period for this period_date has not been created yet"
	}
	if errors.Is(err, ErrFiscalPeriodClosed) {
		return "fiscal period for this period_date is closed"
	}
	return err.Error()
}

func (r *FixedAssetRepository) postOneDepreciationRow(
	ctx context.Context, scheduleID, fixedAssetID string, periodDate time.Time, amount, expenseAccountID, accumAccountID string,
	entryNumbers func(year int) (string, error), actorID *string,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	entryNumber, err := entryNumbers(periodDate.Year())
	if err != nil {
		return err
	}

	entryID, err := InsertJournalEntryTx(ctx, tx, CreateJournalEntryParams{
		EntryNumber: entryNumber,
		EntryDate:   periodDate,
		SourceType:  model.JournalSourceDepreciation,
		SourceID:    &scheduleID,
		Description: "Beban depresiasi periode berjalan",
		CreatedBy:   actorID,
		Lines: []CreateJournalLineParams{
			{AccountID: expenseAccountID, Debit: amount},
			{AccountID: accumAccountID, Credit: amount, SubledgerType: model.SubledgerAsset, SubledgerID: &fixedAssetID},
		},
	})
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		UPDATE finance_depreciation_schedules
		SET status = 'posted', journal_entry_id = $2::uuid, posted_at = $3, updated_at = $3
		WHERE id = $1::uuid
	`, scheduleID, entryID, now); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

type DepreciationPostResult struct {
	ScheduleID string
	AssetCode  string
	PeriodDate time.Time
	Status     string
	Reason     string
}
