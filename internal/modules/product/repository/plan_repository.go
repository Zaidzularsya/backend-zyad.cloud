package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"zyad.cloud/internal/modules/product/model"
	"zyad.cloud/internal/platform/database"

	"github.com/jackc/pgx/v5"
)

const planSelectColumns = `
	id,
	code,
	name,
	COALESCE(description, ''),
	plan_type,
	is_public,
	is_active,
	sort_order,
	metadata,
	created_at,
	updated_at,
	deleted_at
`

const planPriceSelectColumns = `
	id,
	plan_id,
	billing_interval,
	currency,
	amount::text,
	is_active,
	metadata,
	created_at,
	updated_at,
	deleted_at
`

type PlanRepository struct {
	db *database.Pool
}

type PlanListFilter struct {
	Type           model.PlanType
	IsPublic       *bool
	IsActive       *bool
	IncludeDeleted bool
	Search         string
	Limit          int
	Offset         int
}

type CreatePlanParams struct {
	Code        string
	Name        string
	Description string
	Type        model.PlanType
	IsPublic    bool
	IsActive    bool
	SortOrder   int
	Metadata    map[string]any
}

type UpdatePlanParams struct {
	ID          string
	Name        *string
	Description *string
	Type        *model.PlanType
	IsPublic    *bool
	IsActive    *bool
	SortOrder   *int
	Metadata    *map[string]any
}

type UpsertPlanPriceParams struct {
	PlanID          string
	BillingInterval model.BillingInterval
	Currency        string
	Amount          string
	IsActive        bool
	Metadata        map[string]any
}

type CreatePlanPriceParams struct {
	PlanID          string
	BillingInterval model.BillingInterval
	Currency        string
	Amount          string
	IsActive        bool
	Metadata        map[string]any
}

type UpdatePlanPriceParams struct {
	ID              string
	PlanID          string
	BillingInterval *model.BillingInterval
	Currency        *string
	Amount          *string
	IsActive        *bool
	Metadata        *map[string]any
}

func NewPlanRepository(db *database.Pool) *PlanRepository {
	return &PlanRepository{db: db}
}

func (r *PlanRepository) Create(ctx context.Context, params CreatePlanParams) (model.Plan, error) {
	metadata, err := encodeMap(params.Metadata)
	if err != nil {
		return model.Plan{}, err
	}
	var plan model.Plan
	var metadataBytes []byte
	err = r.db.QueryRow(ctx, `
		INSERT INTO product_plans (
			code,
			name,
			description,
			plan_type,
			is_public,
			is_active,
			sort_order,
			metadata
		)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8::jsonb)
		RETURNING `+planSelectColumns,
		canonicalKey(params.Code),
		strings.TrimSpace(params.Name),
		strings.TrimSpace(params.Description),
		string(params.Type),
		params.IsPublic,
		params.IsActive,
		params.SortOrder,
		metadata,
	).Scan(planScanDest(&plan, &metadataBytes)...)
	if err != nil {
		return model.Plan{}, err
	}
	if err := decodeMap(metadataBytes, &plan.Metadata); err != nil {
		return model.Plan{}, err
	}
	return plan, nil
}

func (r *PlanRepository) FindByID(ctx context.Context, id string) (model.Plan, error) {
	return r.find(ctx, "id = $1::uuid AND deleted_at IS NULL", strings.TrimSpace(id))
}

func (r *PlanRepository) FindByCode(ctx context.Context, code string) (model.Plan, error) {
	return r.find(ctx, "code = $1 AND deleted_at IS NULL", canonicalKey(code))
}

func (r *PlanRepository) find(ctx context.Context, where string, args ...any) (model.Plan, error) {
	var plan model.Plan
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		SELECT `+planSelectColumns+`
		FROM product_plans
		WHERE `+where+`
	`, args...).Scan(planScanDest(&plan, &metadataBytes)...)
	if err != nil {
		return model.Plan{}, err
	}
	if err := decodeMap(metadataBytes, &plan.Metadata); err != nil {
		return model.Plan{}, err
	}
	return plan, nil
}

func (r *PlanRepository) List(ctx context.Context, filter PlanListFilter) ([]model.Plan, int64, error) {
	where, args := planWhere(filter)
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM product_plans"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := pagination(filter.Limit, filter.Offset)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+planSelectColumns+`
		FROM product_plans`+where+`
		ORDER BY sort_order ASC, created_at ASC, id ASC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	plans := make([]model.Plan, 0)
	for rows.Next() {
		var plan model.Plan
		var metadataBytes []byte
		if err := rows.Scan(planScanDest(&plan, &metadataBytes)...); err != nil {
			return nil, 0, err
		}
		if err := decodeMap(metadataBytes, &plan.Metadata); err != nil {
			return nil, 0, err
		}
		plans = append(plans, plan)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return plans, total, nil
}

func (r *PlanRepository) Update(ctx context.Context, params UpdatePlanParams) (model.Plan, error) {
	var plan model.Plan
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		UPDATE product_plans
		SET
			name = COALESCE(NULLIF($2, ''), name),
			description = CASE WHEN $3::boolean THEN NULLIF($4, '') ELSE description END,
			plan_type = COALESCE(NULLIF($5, ''), plan_type),
			is_public = COALESCE($6, is_public),
			is_active = COALESCE($7, is_active),
			sort_order = COALESCE($8, sort_order),
			metadata = COALESCE($9::jsonb, metadata),
			updated_at = now()
		WHERE id = $1::uuid
			AND deleted_at IS NULL
		RETURNING `+planSelectColumns,
		strings.TrimSpace(params.ID),
		stringValue(params.Name),
		params.Description != nil,
		stringValue(params.Description),
		planTypeValue(params.Type),
		params.IsPublic,
		params.IsActive,
		params.SortOrder,
		optionalMap(params.Metadata),
	).Scan(planScanDest(&plan, &metadataBytes)...)
	if err != nil {
		return model.Plan{}, err
	}
	if err := decodeMap(metadataBytes, &plan.Metadata); err != nil {
		return model.Plan{}, err
	}
	return plan, nil
}

func (r *PlanRepository) SoftDelete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE product_plans
		SET deleted_at = COALESCE(deleted_at, now()), updated_at = now()
		WHERE id = $1::uuid
	`, strings.TrimSpace(id))
	return err
}

func (r *PlanRepository) UpsertPrice(ctx context.Context, params UpsertPlanPriceParams) (model.PlanPrice, error) {
	metadata, err := encodeMap(params.Metadata)
	if err != nil {
		return model.PlanPrice{}, err
	}
	var price model.PlanPrice
	var metadataBytes []byte
	err = r.db.QueryRow(ctx, `
		INSERT INTO product_plan_prices (
			plan_id,
			billing_interval,
			currency,
			amount,
			is_active,
			metadata
		)
		VALUES ($1::uuid, $2, $3, $4::numeric, $5, $6::jsonb)
		ON CONFLICT (plan_id, billing_interval, currency)
		WHERE deleted_at IS NULL
		DO UPDATE SET
			amount = EXCLUDED.amount,
			is_active = EXCLUDED.is_active,
			metadata = EXCLUDED.metadata,
			updated_at = now()
		RETURNING `+planPriceSelectColumns,
		strings.TrimSpace(params.PlanID),
		string(params.BillingInterval),
		upperOrDefault(params.Currency, "IDR"),
		strings.TrimSpace(params.Amount),
		params.IsActive,
		metadata,
	).Scan(planPriceScanDest(&price, &metadataBytes)...)
	if err != nil {
		return model.PlanPrice{}, err
	}
	if err := decodeMap(metadataBytes, &price.Metadata); err != nil {
		return model.PlanPrice{}, err
	}
	return price, nil
}

func (r *PlanRepository) CreatePrice(ctx context.Context, params CreatePlanPriceParams) (model.PlanPrice, error) {
	metadata, err := encodeMap(params.Metadata)
	if err != nil {
		return model.PlanPrice{}, err
	}
	var price model.PlanPrice
	var metadataBytes []byte
	err = r.db.QueryRow(ctx, `
		INSERT INTO product_plan_prices (
			plan_id,
			billing_interval,
			currency,
			amount,
			is_active,
			metadata
		)
		VALUES ($1::uuid, $2, $3, $4::numeric, $5, $6::jsonb)
		RETURNING `+planPriceSelectColumns,
		strings.TrimSpace(params.PlanID),
		string(params.BillingInterval),
		upperOrDefault(params.Currency, "IDR"),
		strings.TrimSpace(params.Amount),
		params.IsActive,
		metadata,
	).Scan(planPriceScanDest(&price, &metadataBytes)...)
	if err != nil {
		return model.PlanPrice{}, err
	}
	if err := decodeMap(metadataBytes, &price.Metadata); err != nil {
		return model.PlanPrice{}, err
	}
	return price, nil
}

func (r *PlanRepository) ListPrices(ctx context.Context, planID string, includeDeleted bool) ([]model.PlanPrice, error) {
	where := "plan_id = $1::uuid"
	if !includeDeleted {
		where += " AND deleted_at IS NULL"
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+planPriceSelectColumns+`
		FROM product_plan_prices
		WHERE `+where+`
		ORDER BY billing_interval ASC, currency ASC
	`, strings.TrimSpace(planID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prices := make([]model.PlanPrice, 0)
	for rows.Next() {
		var price model.PlanPrice
		var metadataBytes []byte
		if err := rows.Scan(planPriceScanDest(&price, &metadataBytes)...); err != nil {
			return nil, err
		}
		if err := decodeMap(metadataBytes, &price.Metadata); err != nil {
			return nil, err
		}
		prices = append(prices, price)
	}
	return prices, rows.Err()
}

func (r *PlanRepository) FindPriceByID(
	ctx context.Context,
	planID string,
	priceID string,
	includeDeleted bool,
) (model.PlanPrice, error) {
	where := "id = $1::uuid AND plan_id = $2::uuid"
	if !includeDeleted {
		where += " AND deleted_at IS NULL"
	}

	var price model.PlanPrice
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		SELECT `+planPriceSelectColumns+`
		FROM product_plan_prices
		WHERE `+where,
		strings.TrimSpace(priceID),
		strings.TrimSpace(planID),
	).Scan(planPriceScanDest(&price, &metadataBytes)...)
	if err != nil {
		return model.PlanPrice{}, err
	}
	if err := decodeMap(metadataBytes, &price.Metadata); err != nil {
		return model.PlanPrice{}, err
	}
	return price, nil
}

func (r *PlanRepository) UpdatePrice(ctx context.Context, params UpdatePlanPriceParams) (model.PlanPrice, error) {
	var price model.PlanPrice
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		UPDATE product_plan_prices
		SET
			billing_interval = COALESCE(NULLIF($3, ''), billing_interval),
			currency = COALESCE(NULLIF($4, ''), currency),
			amount = COALESCE(NULLIF($5, '')::numeric, amount),
			is_active = COALESCE($6, is_active),
			metadata = COALESCE($7::jsonb, metadata),
			updated_at = now()
		WHERE id = $1::uuid
			AND plan_id = $2::uuid
			AND deleted_at IS NULL
		RETURNING `+planPriceSelectColumns,
		strings.TrimSpace(params.ID),
		strings.TrimSpace(params.PlanID),
		billingIntervalValue(params.BillingInterval),
		upperOptionalString(params.Currency),
		stringValue(params.Amount),
		params.IsActive,
		optionalMap(params.Metadata),
	).Scan(planPriceScanDest(&price, &metadataBytes)...)
	if err != nil {
		return model.PlanPrice{}, err
	}
	if err := decodeMap(metadataBytes, &price.Metadata); err != nil {
		return model.PlanPrice{}, err
	}
	return price, nil
}

func (r *PlanRepository) SoftDeletePrice(ctx context.Context, planID string, priceID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE product_plan_prices
		SET deleted_at = COALESCE(deleted_at, now()), updated_at = now()
		WHERE id = $1::uuid
			AND plan_id = $2::uuid
			AND deleted_at IS NULL
	`, strings.TrimSpace(priceID), strings.TrimSpace(planID))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w", pgx.ErrNoRows)
	}
	return nil
}

func planWhere(filter PlanListFilter) (string, []any) {
	conditions := []string{}
	args := []any{}
	if !filter.IncludeDeleted {
		conditions = append(conditions, "deleted_at IS NULL")
	}
	if filter.Type != "" {
		args = append(args, string(filter.Type))
		conditions = append(conditions, fmt.Sprintf("plan_type = $%d", len(args)))
	}
	if filter.IsPublic != nil {
		args = append(args, *filter.IsPublic)
		conditions = append(conditions, fmt.Sprintf("is_public = $%d", len(args)))
	}
	if filter.IsActive != nil {
		args = append(args, *filter.IsActive)
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", len(args)))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		conditions = append(conditions, fmt.Sprintf("(lower(code) LIKE $%d OR lower(name) LIKE $%d)", len(args), len(args)))
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func planScanDest(plan *model.Plan, metadata *[]byte) []any {
	return []any{
		&plan.ID,
		&plan.Code,
		&plan.Name,
		&plan.Description,
		&plan.Type,
		&plan.IsPublic,
		&plan.IsActive,
		&plan.SortOrder,
		metadata,
		&plan.CreatedAt,
		&plan.UpdatedAt,
		&plan.DeletedAt,
	}
}

func planPriceScanDest(price *model.PlanPrice, metadata *[]byte) []any {
	return []any{
		&price.ID,
		&price.PlanID,
		&price.BillingInterval,
		&price.Currency,
		&price.Amount,
		&price.IsActive,
		metadata,
		&price.CreatedAt,
		&price.UpdatedAt,
		&price.DeletedAt,
	}
}

func encodeMap(value map[string]any) (string, error) {
	if value == nil {
		value = map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func decodeMap(data []byte, target *map[string]any) error {
	if len(data) == 0 {
		*target = map[string]any{}
		return nil
	}
	return json.Unmarshal(data, target)
}

func optionalMap(value *map[string]any) any {
	if value == nil {
		return nil
	}
	encoded, err := encodeMap(*value)
	if err != nil {
		return nil
	}
	return encoded
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func planTypeValue(value *model.PlanType) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func canonicalKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func upperOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(*value))
}

func upperOrDefault(value string, fallback string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func pagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func billingIntervalValue(value *model.BillingInterval) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

var _ = time.Time{}
