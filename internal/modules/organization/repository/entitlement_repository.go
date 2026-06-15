package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database"
)

var ErrUsageLimitExceeded = errors.New("organization usage limit exceeded")

const entitlementSelectColumns = `
	id,
	organization_id,
	feature_key,
	source,
	COALESCE(source_reference, ''),
	status,
	limits,
	version,
	effective_from,
	effective_until,
	COALESCE(reason, ''),
	COALESCE(created_by::text, ''),
	COALESCE(updated_by::text, ''),
	created_at,
	updated_at
`

const usageCounterSelectColumns = `
	id,
	organization_id,
	feature_key,
	metric_key,
	period_start,
	period_end,
	usage_value,
	version,
	last_recorded_at,
	created_at,
	updated_at
`

type EntitlementRepository struct {
	db *database.Pool
}

type UpsertEntitlementParams struct {
	OrganizationID  string
	FeatureKey      string
	Source          model.EntitlementSource
	SourceReference string
	Status          model.EntitlementStatus
	Limits          map[string]any
	EffectiveFrom   time.Time
	EffectiveUntil  *time.Time
	Reason          string
	ActorUserID     string
}

type EntitlementListFilter struct {
	FeatureKey string
	Source     model.EntitlementSource
	Status     model.EntitlementStatus
	Limit      int
	Offset     int
}

type UsageCounterKey struct {
	OrganizationID string
	FeatureKey     string
	MetricKey      string
	PeriodStart    time.Time
	PeriodEnd      time.Time
}

type EntitlementVersionMetadata struct {
	RowCount        int64
	MaxVersion      int64
	LatestUpdatedAt *time.Time
}

func NewEntitlementRepository(db *database.Pool) *EntitlementRepository {
	return &EntitlementRepository{db: db}
}

func (r *EntitlementRepository) Upsert(
	ctx context.Context,
	params UpsertEntitlementParams,
) (model.Entitlement, error) {
	limits, err := encodeLimits(params.Limits)
	if err != nil {
		return model.Entitlement{}, err
	}
	status := params.Status
	if status == "" {
		status = model.EntitlementStatusActive
	}
	effectiveFrom := params.EffectiveFrom
	if effectiveFrom.IsZero() {
		effectiveFrom = time.Now().UTC()
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.Entitlement{}, err
	}
	defer tx.Rollback(ctx)

	organizationID := strings.TrimSpace(params.OrganizationID)
	featureKey := canonicalFeatureKey(params.FeatureKey)
	sourceReference := strings.TrimSpace(params.SourceReference)
	if err := lockEntitlementKey(
		ctx,
		tx,
		organizationID,
		featureKey,
		params.Source,
		sourceReference,
	); err != nil {
		return model.Entitlement{}, err
	}

	var entitlement model.Entitlement
	var limitsBytes []byte
	err = tx.QueryRow(ctx, `
		SELECT `+entitlementSelectColumns+`
		FROM organization_entitlements
		WHERE organization_id = $1::uuid
			AND feature_key = $2
			AND source = $3
			AND source_reference IS NOT DISTINCT FROM NULLIF($4, '')
		ORDER BY version DESC, updated_at DESC, id DESC
		LIMIT 1
		FOR UPDATE
	`, organizationID, featureKey, string(params.Source), sourceReference).
		Scan(entitlementScanDest(&entitlement, &limitsBytes)...)
	switch {
	case err == nil:
		err = tx.QueryRow(ctx, `
			UPDATE organization_entitlements
			SET
				status = $2,
				limits = $3::jsonb,
				effective_from = $4,
				effective_until = $5,
				reason = NULLIF($6, ''),
				updated_by = NULLIF($7, '')::uuid,
				updated_at = now()
			WHERE id = $1::uuid
			RETURNING `+entitlementSelectColumns,
			entitlement.ID,
			string(status),
			string(limits),
			effectiveFrom,
			params.EffectiveUntil,
			strings.TrimSpace(params.Reason),
			strings.TrimSpace(params.ActorUserID),
		).Scan(entitlementScanDest(&entitlement, &limitsBytes)...)
	case errors.Is(err, pgx.ErrNoRows):
		err = tx.QueryRow(ctx, `
			INSERT INTO organization_entitlements (
				organization_id,
				feature_key,
				source,
				source_reference,
				status,
				limits,
				effective_from,
				effective_until,
				reason,
				created_by,
				updated_by
			)
			SELECT
				organization.id,
				$2,
				$3,
				NULLIF($4, ''),
				$5,
				$6::jsonb,
				$7,
				$8,
				NULLIF($9, ''),
				NULLIF($10, '')::uuid,
				NULLIF($10, '')::uuid
			FROM organizations organization
			WHERE organization.id = $1::uuid
				AND organization.deleted_at IS NULL
				AND organization.status <> 'archived'
			RETURNING `+entitlementSelectColumns,
			organizationID,
			featureKey,
			string(params.Source),
			sourceReference,
			string(status),
			string(limits),
			effectiveFrom,
			params.EffectiveUntil,
			strings.TrimSpace(params.Reason),
			strings.TrimSpace(params.ActorUserID),
		).Scan(entitlementScanDest(&entitlement, &limitsBytes)...)
	}
	if err != nil {
		return model.Entitlement{}, err
	}
	if err := decodeLimits(limitsBytes, &entitlement.Limits); err != nil {
		return model.Entitlement{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Entitlement{}, err
	}
	return entitlement, nil
}

func (r *EntitlementRepository) FindEffective(
	ctx context.Context,
	organizationID string,
	featureKey string,
	at time.Time,
) (model.Entitlement, error) {
	var entitlement model.Entitlement
	var limitsBytes []byte
	err := r.db.QueryRow(ctx, `
		SELECT `+entitlementSelectColumns+`
		FROM organization_entitlements
		WHERE organization_id = $1::uuid
			AND feature_key = $2
			AND status = 'active'
			AND effective_from <= $3
			AND (effective_until IS NULL OR effective_until > $3)
		ORDER BY
			CASE source
				WHEN 'platform_override' THEN 4
				WHEN 'addon' THEN 3
				WHEN 'trial' THEN 2
				WHEN 'plan' THEN 1
			END DESC,
			version DESC,
			effective_from DESC,
			updated_at DESC,
			id DESC
		LIMIT 1
	`, strings.TrimSpace(organizationID), canonicalFeatureKey(featureKey), at).
		Scan(entitlementScanDest(&entitlement, &limitsBytes)...)
	if err != nil {
		return model.Entitlement{}, err
	}
	if err := decodeLimits(limitsBytes, &entitlement.Limits); err != nil {
		return model.Entitlement{}, err
	}
	return entitlement, nil
}

func (r *EntitlementRepository) List(
	ctx context.Context,
	organizationID string,
	filter EntitlementListFilter,
) ([]model.Entitlement, int64, error) {
	where, args := entitlementWhere(filter)
	args = append([]any{strings.TrimSpace(organizationID)}, args...)
	where = " WHERE organization_id = $1::uuid" + renumberWhere(where, 1)

	var total int64
	if err := r.db.QueryRow(
		ctx,
		"SELECT count(*) FROM organization_entitlements"+where,
		args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := entitlementPagination(filter)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+entitlementSelectColumns+`
		FROM organization_entitlements`+where+`
		ORDER BY feature_key ASC, effective_from DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entitlements := make([]model.Entitlement, 0)
	for rows.Next() {
		var entitlement model.Entitlement
		var limitsBytes []byte
		if err := rows.Scan(entitlementScanDest(&entitlement, &limitsBytes)...); err != nil {
			return nil, 0, err
		}
		if err := decodeLimits(limitsBytes, &entitlement.Limits); err != nil {
			return nil, 0, err
		}
		entitlements = append(entitlements, entitlement)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return entitlements, total, nil
}

func (r *EntitlementRepository) VersionMetadata(
	ctx context.Context,
	organizationID string,
) (EntitlementVersionMetadata, error) {
	var metadata EntitlementVersionMetadata
	err := r.db.QueryRow(ctx, `
		SELECT count(*), COALESCE(max(version), 0), max(updated_at)
		FROM organization_entitlements
		WHERE organization_id = $1::uuid
	`, strings.TrimSpace(organizationID)).Scan(
		&metadata.RowCount,
		&metadata.MaxVersion,
		&metadata.LatestUpdatedAt,
	)
	return metadata, err
}

func (r *EntitlementRepository) GetUsage(
	ctx context.Context,
	key UsageCounterKey,
) (model.UsageCounter, error) {
	var counter model.UsageCounter
	err := r.db.QueryRow(ctx, `
		SELECT `+usageCounterSelectColumns+`
		FROM organization_usage_counters
		WHERE organization_id = $1::uuid
			AND feature_key = $2
			AND metric_key = $3
			AND period_start = $4
			AND period_end = $5
	`, strings.TrimSpace(key.OrganizationID), canonicalFeatureKey(key.FeatureKey),
		canonicalMetricKey(key.MetricKey), key.PeriodStart, key.PeriodEnd).
		Scan(usageCounterScanDest(&counter)...)
	if err != nil {
		return model.UsageCounter{}, err
	}
	return counter, nil
}

func (r *EntitlementRepository) IncrementUsage(
	ctx context.Context,
	key UsageCounterKey,
	delta int64,
	maxValue *int64,
	recordedAt time.Time,
) (model.UsageCounter, error) {
	if delta <= 0 {
		return model.UsageCounter{}, errors.New("usage delta must be greater than zero")
	}

	var counter model.UsageCounter
	err := r.db.QueryRow(ctx, `
		INSERT INTO organization_usage_counters (
			organization_id,
			feature_key,
			metric_key,
			period_start,
			period_end,
			usage_value,
			last_recorded_at
		)
		SELECT
			organization.id,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7
		FROM organizations organization
		WHERE organization.id = $1::uuid
			AND organization.deleted_at IS NULL
			AND organization.status <> 'archived'
			AND ($8::bigint IS NULL OR $6 <= $8)
		ON CONFLICT (
			organization_id,
			feature_key,
			metric_key,
			period_start,
			period_end
		)
		DO UPDATE SET
			usage_value = organization_usage_counters.usage_value + EXCLUDED.usage_value,
			last_recorded_at = EXCLUDED.last_recorded_at,
			updated_at = now()
		WHERE $8::bigint IS NULL
			OR organization_usage_counters.usage_value + EXCLUDED.usage_value <= $8
		RETURNING `+usageCounterSelectColumns,
		strings.TrimSpace(key.OrganizationID),
		canonicalFeatureKey(key.FeatureKey),
		canonicalMetricKey(key.MetricKey),
		key.PeriodStart,
		key.PeriodEnd,
		delta,
		recordedAt,
		maxValue,
	).Scan(usageCounterScanDest(&counter)...)
	if errors.Is(err, pgx.ErrNoRows) && maxValue != nil {
		if delta > *maxValue {
			return model.UsageCounter{}, ErrUsageLimitExceeded
		}
		var organizationExists bool
		existsErr := r.db.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM organizations
				WHERE id = $1::uuid
					AND deleted_at IS NULL
					AND status <> 'archived'
			)
		`, strings.TrimSpace(key.OrganizationID)).Scan(&organizationExists)
		if existsErr != nil {
			return model.UsageCounter{}, existsErr
		}
		if organizationExists {
			return model.UsageCounter{}, ErrUsageLimitExceeded
		}
	}
	if err != nil {
		return model.UsageCounter{}, err
	}
	return counter, nil
}

func lockEntitlementKey(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	featureKey string,
	source model.EntitlementSource,
	sourceReference string,
) error {
	key := strings.Join([]string{
		organizationID,
		featureKey,
		string(source),
		sourceReference,
	}, ":")
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	return err
}

func entitlementWhere(filter EntitlementListFilter) (string, []any) {
	var query strings.Builder
	query.WriteString(" WHERE 1 = 1")
	args := make([]any, 0, 3)
	if filter.FeatureKey != "" {
		args = append(args, canonicalFeatureKey(filter.FeatureKey))
		query.WriteString(fmt.Sprintf(" AND feature_key = $%d", len(args)))
	}
	if filter.Source != "" {
		args = append(args, string(filter.Source))
		query.WriteString(fmt.Sprintf(" AND source = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		query.WriteString(fmt.Sprintf(" AND status = $%d", len(args)))
	}
	return query.String(), args
}

func entitlementPagination(filter EntitlementListFilter) (int, int) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func entitlementScanDest(entitlement *model.Entitlement, limitsBytes *[]byte) []any {
	return []any{
		&entitlement.ID,
		&entitlement.OrganizationID,
		&entitlement.FeatureKey,
		&entitlement.Source,
		&entitlement.SourceReference,
		&entitlement.Status,
		limitsBytes,
		&entitlement.Version,
		&entitlement.EffectiveFrom,
		&entitlement.EffectiveUntil,
		&entitlement.Reason,
		&entitlement.CreatedBy,
		&entitlement.UpdatedBy,
		&entitlement.CreatedAt,
		&entitlement.UpdatedAt,
	}
}

func usageCounterScanDest(counter *model.UsageCounter) []any {
	return []any{
		&counter.ID,
		&counter.OrganizationID,
		&counter.FeatureKey,
		&counter.MetricKey,
		&counter.PeriodStart,
		&counter.PeriodEnd,
		&counter.UsageValue,
		&counter.Version,
		&counter.LastRecordedAt,
		&counter.CreatedAt,
		&counter.UpdatedAt,
	}
}

func encodeLimits(limits map[string]any) ([]byte, error) {
	if limits == nil {
		limits = map[string]any{}
	}
	return json.Marshal(limits)
}

func decodeLimits(data []byte, limits *map[string]any) error {
	if len(data) == 0 {
		*limits = map[string]any{}
		return nil
	}
	return json.Unmarshal(data, limits)
}

func canonicalFeatureKey(featureKey string) string {
	return strings.ToLower(strings.TrimSpace(featureKey))
}

func canonicalMetricKey(metricKey string) string {
	return strings.ToLower(strings.TrimSpace(metricKey))
}

func renumberWhere(where string, offset int) string {
	for index := 10; index >= 1; index-- {
		where = strings.ReplaceAll(
			where,
			fmt.Sprintf("$%d", index),
			fmt.Sprintf("$%d", index+offset),
		)
	}
	return strings.TrimPrefix(where, " WHERE 1 = 1")
}
