package repository

import (
	"context"
	"fmt"
	"strings"

	"zyad.cloud/internal/modules/product/model"
	"zyad.cloud/internal/platform/database"
)

const featureSelectColumns = `
	id,
	feature_key,
	module,
	name,
	COALESCE(description, ''),
	value_type,
	COALESCE(unit, ''),
	reset_strategy,
	is_active,
	created_at,
	updated_at
`

type FeatureRepository struct {
	db *database.Pool
}

type FeatureListFilter struct {
	Module   string
	IsActive *bool
	Search   string
	Limit    int
	Offset   int
}

type CreateFeatureParams struct {
	Key           string
	Module        string
	Name          string
	Description   string
	ValueType     model.FeatureValueType
	Unit          string
	ResetStrategy model.ResetStrategy
	IsActive      bool
}

type UpdateFeatureParams struct {
	ID            string
	Module        *string
	Name          *string
	Description   *string
	ValueType     *model.FeatureValueType
	Unit          *string
	ResetStrategy *model.ResetStrategy
	IsActive      *bool
}

func NewFeatureRepository(db *database.Pool) *FeatureRepository {
	return &FeatureRepository{db: db}
}

func (r *FeatureRepository) Create(ctx context.Context, params CreateFeatureParams) (model.Feature, error) {
	resetStrategy := params.ResetStrategy
	if resetStrategy == "" {
		resetStrategy = model.ResetStrategyNever
	}
	var feature model.Feature
	err := r.db.QueryRow(ctx, `
		INSERT INTO product_features (
			feature_key,
			module,
			name,
			description,
			value_type,
			unit,
			reset_strategy,
			is_active
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''), $7, $8)
		RETURNING `+featureSelectColumns,
		canonicalKey(params.Key),
		canonicalKey(params.Module),
		strings.TrimSpace(params.Name),
		strings.TrimSpace(params.Description),
		string(params.ValueType),
		strings.TrimSpace(params.Unit),
		string(resetStrategy),
		params.IsActive,
	).Scan(featureScanDest(&feature)...)
	if err != nil {
		return model.Feature{}, err
	}
	return feature, nil
}

func (r *FeatureRepository) FindByID(ctx context.Context, id string) (model.Feature, error) {
	return r.find(ctx, "id = $1::uuid", strings.TrimSpace(id))
}

func (r *FeatureRepository) FindByKey(ctx context.Context, key string) (model.Feature, error) {
	return r.find(ctx, "feature_key = $1", canonicalKey(key))
}

func (r *FeatureRepository) find(ctx context.Context, where string, args ...any) (model.Feature, error) {
	var feature model.Feature
	err := r.db.QueryRow(ctx, `
		SELECT `+featureSelectColumns+`
		FROM product_features
		WHERE `+where+`
	`, args...).Scan(featureScanDest(&feature)...)
	if err != nil {
		return model.Feature{}, err
	}
	return feature, nil
}

func (r *FeatureRepository) List(ctx context.Context, filter FeatureListFilter) ([]model.Feature, int64, error) {
	where, args := featureWhere(filter)
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM product_features"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := pagination(filter.Limit, filter.Offset)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+featureSelectColumns+`
		FROM product_features`+where+`
		ORDER BY module ASC, feature_key ASC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	features := make([]model.Feature, 0)
	for rows.Next() {
		var feature model.Feature
		if err := rows.Scan(featureScanDest(&feature)...); err != nil {
			return nil, 0, err
		}
		features = append(features, feature)
	}
	return features, total, rows.Err()
}

func (r *FeatureRepository) Update(ctx context.Context, params UpdateFeatureParams) (model.Feature, error) {
	var feature model.Feature
	err := r.db.QueryRow(ctx, `
		UPDATE product_features
		SET
			module = COALESCE(NULLIF($2, ''), module),
			name = COALESCE(NULLIF($3, ''), name),
			description = CASE WHEN $4::boolean THEN NULLIF($5, '') ELSE description END,
			value_type = COALESCE(NULLIF($6, ''), value_type),
			unit = CASE WHEN $7::boolean THEN NULLIF($8, '') ELSE unit END,
			reset_strategy = COALESCE(NULLIF($9, ''), reset_strategy),
			is_active = COALESCE($10, is_active),
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING `+featureSelectColumns,
		strings.TrimSpace(params.ID),
		stringValue(params.Module),
		stringValue(params.Name),
		params.Description != nil,
		stringValue(params.Description),
		featureValueTypeValue(params.ValueType),
		params.Unit != nil,
		stringValue(params.Unit),
		resetStrategyValue(params.ResetStrategy),
		params.IsActive,
	).Scan(featureScanDest(&feature)...)
	if err != nil {
		return model.Feature{}, err
	}
	return feature, nil
}

func featureWhere(filter FeatureListFilter) (string, []any) {
	conditions := []string{}
	args := []any{}
	if filter.Module != "" {
		args = append(args, canonicalKey(filter.Module))
		conditions = append(conditions, fmt.Sprintf("module = $%d", len(args)))
	}
	if filter.IsActive != nil {
		args = append(args, *filter.IsActive)
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", len(args)))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		conditions = append(conditions, fmt.Sprintf("(feature_key LIKE $%d OR lower(name) LIKE $%d)", len(args), len(args)))
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func featureScanDest(feature *model.Feature) []any {
	return []any{
		&feature.ID,
		&feature.Key,
		&feature.Module,
		&feature.Name,
		&feature.Description,
		&feature.ValueType,
		&feature.Unit,
		&feature.ResetStrategy,
		&feature.IsActive,
		&feature.CreatedAt,
		&feature.UpdatedAt,
	}
}

func featureValueTypeValue(value *model.FeatureValueType) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func resetStrategyValue(value *model.ResetStrategy) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
