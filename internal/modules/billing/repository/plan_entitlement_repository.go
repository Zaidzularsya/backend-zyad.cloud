package repository

import (
	"context"
	"strings"

	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/platform/database"
)

const planEntitlementSelectColumns = `
	entitlement.id,
	entitlement.plan_id,
	entitlement.feature_id,
	feature.feature_key,
	entitlement.value_bool,
	entitlement.value_int,
	entitlement.value_decimal::text,
	entitlement.value_string,
	entitlement.limits,
	entitlement.created_at,
	entitlement.updated_at
`

type PlanEntitlementRepository struct {
	db *database.Pool
}

type UpsertPlanEntitlementParams struct {
	FeatureID    string
	ValueBool    *bool
	ValueInt     *int64
	ValueDecimal *string
	ValueString  *string
	Limits       map[string]any
}

func NewPlanEntitlementRepository(db *database.Pool) *PlanEntitlementRepository {
	return &PlanEntitlementRepository{db: db}
}

func (r *PlanEntitlementRepository) ListByPlanID(ctx context.Context, planID string) ([]model.PlanEntitlement, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+planEntitlementSelectColumns+`
		FROM billing_plan_entitlements entitlement
		JOIN billing_features feature ON feature.id = entitlement.feature_id
		WHERE entitlement.plan_id = $1::uuid
		ORDER BY feature.module ASC, feature.feature_key ASC
	`, strings.TrimSpace(planID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entitlements := make([]model.PlanEntitlement, 0)
	for rows.Next() {
		entitlement, err := scanPlanEntitlement(rows.Scan)
		if err != nil {
			return nil, err
		}
		entitlements = append(entitlements, entitlement)
	}
	return entitlements, rows.Err()
}

func (r *PlanEntitlementRepository) ReplaceByPlanID(
	ctx context.Context,
	planID string,
	entitlements []UpsertPlanEntitlementParams,
) ([]model.PlanEntitlement, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM billing_plan_entitlements
		WHERE plan_id = $1::uuid
	`, strings.TrimSpace(planID)); err != nil {
		return nil, err
	}

	for _, entitlement := range entitlements {
		limits, err := encodeMap(entitlement.Limits)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO billing_plan_entitlements (
				plan_id,
				feature_id,
				value_bool,
				value_int,
				value_decimal,
				value_string,
				limits
			)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5::numeric, $6, $7::jsonb)
		`,
			strings.TrimSpace(planID),
			strings.TrimSpace(entitlement.FeatureID),
			entitlement.ValueBool,
			entitlement.ValueInt,
			decimalValue(entitlement.ValueDecimal),
			stringPointerValue(entitlement.ValueString),
			limits,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.ListByPlanID(ctx, planID)
}

func scanPlanEntitlement(scan func(dest ...any) error) (model.PlanEntitlement, error) {
	var entitlement model.PlanEntitlement
	var limitsBytes []byte
	if err := scan(
		&entitlement.ID,
		&entitlement.PlanID,
		&entitlement.FeatureID,
		&entitlement.FeatureKey,
		&entitlement.ValueBool,
		&entitlement.ValueInt,
		&entitlement.ValueDecimal,
		&entitlement.ValueString,
		&limitsBytes,
		&entitlement.CreatedAt,
		&entitlement.UpdatedAt,
	); err != nil {
		return model.PlanEntitlement{}, err
	}
	if err := decodeMap(limitsBytes, &entitlement.Limits); err != nil {
		return model.PlanEntitlement{}, err
	}
	return entitlement, nil
}

func decimalValue(value *string) any {
	if value == nil {
		return nil
	}
	return strings.TrimSpace(*value)
}

func stringPointerValue(value *string) any {
	if value == nil {
		return nil
	}
	return strings.TrimSpace(*value)
}
