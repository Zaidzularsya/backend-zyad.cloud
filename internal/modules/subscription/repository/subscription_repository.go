package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"zyad.cloud/internal/modules/subscription/model"
	"zyad.cloud/internal/platform/database"
)

const subscriptionSelectColumns = `
	id,
	organization_id,
	plan_id,
	status,
	billing_interval,
	current_period_start,
	current_period_end,
	trial_start,
	trial_end,
	cancel_at_period_end,
	canceled_at,
	suspended_at,
	metadata,
	created_at,
	updated_at
`

type SubscriptionRepository struct {
	db *database.Pool
}

type SubscriptionListFilter struct {
	OrganizationID string
	PlanID         string
	Status         model.SubscriptionStatus
	Limit          int
	Offset         int
}

type CreateSubscriptionParams struct {
	OrganizationID     string
	PlanID             string
	Status             model.SubscriptionStatus
	BillingInterval    model.BillingInterval
	CurrentPeriodStart *time.Time
	CurrentPeriodEnd   *time.Time
	TrialStart         *time.Time
	TrialEnd           *time.Time
	CancelAtPeriodEnd  bool
	CanceledAt         *time.Time
	SuspendedAt        *time.Time
	Metadata           map[string]any
}

type UpdateSubscriptionParams struct {
	ID                 string
	OrganizationID     string
	PlanID             *string
	Status             *model.SubscriptionStatus
	BillingInterval    *model.BillingInterval
	CurrentPeriodStart *time.Time
	CurrentPeriodEnd   *time.Time
	TrialStart         *time.Time
	TrialEnd           *time.Time
	CancelAtPeriodEnd  *bool
	CanceledAt         *time.Time
	SuspendedAt        *time.Time
	Metadata           *map[string]any
}

type SubscriptionEventParams struct {
	SubscriptionID string
	OrganizationID string
	Type           string
	OldStatus      *model.SubscriptionStatus
	NewStatus      *model.SubscriptionStatus
	ActorUserID    *string
	Metadata       map[string]any
}

func NewSubscriptionRepository(db *database.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{db: db}
}

func (r *SubscriptionRepository) Create(ctx context.Context, params CreateSubscriptionParams) (model.Subscription, error) {
	metadata, err := encodeMap(params.Metadata)
	if err != nil {
		return model.Subscription{}, err
	}
	status := params.Status
	if status == "" {
		status = model.SubscriptionStatusActive
	}
	var subscription model.Subscription
	var metadataBytes []byte
	err = r.db.QueryRow(ctx, `
		INSERT INTO customer_subscriptions (
			organization_id,
			plan_id,
			status,
			billing_interval,
			current_period_start,
			current_period_end,
			trial_start,
			trial_end,
			cancel_at_period_end,
			canceled_at,
			suspended_at,
			metadata
		)
		SELECT
			organization.id,
			$2::uuid,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10,
			$11,
			$12::jsonb
		FROM organizations organization
		WHERE organization.id = $1::uuid
			AND organization.deleted_at IS NULL
			AND organization.status <> 'archived'
		RETURNING `+subscriptionSelectColumns,
		strings.TrimSpace(params.OrganizationID),
		strings.TrimSpace(params.PlanID),
		string(status),
		string(params.BillingInterval),
		params.CurrentPeriodStart,
		params.CurrentPeriodEnd,
		params.TrialStart,
		params.TrialEnd,
		params.CancelAtPeriodEnd,
		params.CanceledAt,
		params.SuspendedAt,
		metadata,
	).Scan(subscriptionScanDest(&subscription, &metadataBytes)...)
	if err != nil {
		return model.Subscription{}, err
	}
	if err := decodeMap(metadataBytes, &subscription.Metadata); err != nil {
		return model.Subscription{}, err
	}
	return subscription, nil
}

func (r *SubscriptionRepository) FindByID(ctx context.Context, organizationID string, id string) (model.Subscription, error) {
	var subscription model.Subscription
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		SELECT `+subscriptionSelectColumns+`
		FROM customer_subscriptions
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
	`, strings.TrimSpace(id), strings.TrimSpace(organizationID)).Scan(subscriptionScanDest(&subscription, &metadataBytes)...)
	if err != nil {
		return model.Subscription{}, err
	}
	if err := decodeMap(metadataBytes, &subscription.Metadata); err != nil {
		return model.Subscription{}, err
	}
	return subscription, nil
}

func (r *SubscriptionRepository) FindByIDUnscoped(ctx context.Context, id string) (model.Subscription, error) {
	var subscription model.Subscription
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		SELECT `+subscriptionSelectColumns+`
		FROM customer_subscriptions
		WHERE id = $1::uuid
	`, strings.TrimSpace(id)).Scan(subscriptionScanDest(&subscription, &metadataBytes)...)
	if err != nil {
		return model.Subscription{}, err
	}
	if err := decodeMap(metadataBytes, &subscription.Metadata); err != nil {
		return model.Subscription{}, err
	}
	return subscription, nil
}

func (r *SubscriptionRepository) FindUsableByOrganization(ctx context.Context, organizationID string) (model.Subscription, error) {
	var subscription model.Subscription
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		SELECT `+subscriptionSelectColumns+`
		FROM customer_subscriptions
		WHERE organization_id = $1::uuid
			AND status IN ('trialing', 'active', 'past_due', 'grace_period')
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, strings.TrimSpace(organizationID)).Scan(subscriptionScanDest(&subscription, &metadataBytes)...)
	if err != nil {
		return model.Subscription{}, err
	}
	if err := decodeMap(metadataBytes, &subscription.Metadata); err != nil {
		return model.Subscription{}, err
	}
	return subscription, nil
}

func (r *SubscriptionRepository) List(ctx context.Context, filter SubscriptionListFilter) ([]model.Subscription, int64, error) {
	where, args := subscriptionWhere(filter)
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM customer_subscriptions"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := pagination(filter.Limit, filter.Offset)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+subscriptionSelectColumns+`
		FROM customer_subscriptions`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	subscriptions := make([]model.Subscription, 0)
	for rows.Next() {
		var subscription model.Subscription
		var metadataBytes []byte
		if err := rows.Scan(subscriptionScanDest(&subscription, &metadataBytes)...); err != nil {
			return nil, 0, err
		}
		if err := decodeMap(metadataBytes, &subscription.Metadata); err != nil {
			return nil, 0, err
		}
		subscriptions = append(subscriptions, subscription)
	}
	return subscriptions, total, rows.Err()
}

func (r *SubscriptionRepository) Update(ctx context.Context, params UpdateSubscriptionParams) (model.Subscription, error) {
	var subscription model.Subscription
	var metadataBytes []byte
	err := r.db.QueryRow(ctx, `
		UPDATE customer_subscriptions
		SET
			plan_id = COALESCE(NULLIF($3, '')::uuid, plan_id),
			status = COALESCE(NULLIF($4, ''), status),
			billing_interval = COALESCE(NULLIF($5, ''), billing_interval),
			current_period_start = CASE WHEN $6::boolean THEN $7 ELSE current_period_start END,
			current_period_end = CASE WHEN $8::boolean THEN $9 ELSE current_period_end END,
			trial_start = CASE WHEN $10::boolean THEN $11 ELSE trial_start END,
			trial_end = CASE WHEN $12::boolean THEN $13 ELSE trial_end END,
			cancel_at_period_end = COALESCE($14, cancel_at_period_end),
			canceled_at = CASE WHEN $15::boolean THEN $16 ELSE canceled_at END,
			suspended_at = CASE WHEN $17::boolean THEN $18 ELSE suspended_at END,
			metadata = COALESCE($19::jsonb, metadata),
			updated_at = now()
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
		RETURNING `+subscriptionSelectColumns,
		strings.TrimSpace(params.ID),
		strings.TrimSpace(params.OrganizationID),
		stringPointerValue(params.PlanID),
		subscriptionStatusValue(params.Status),
		billingIntervalValue(params.BillingInterval),
		params.CurrentPeriodStart != nil,
		params.CurrentPeriodStart,
		params.CurrentPeriodEnd != nil,
		params.CurrentPeriodEnd,
		params.TrialStart != nil,
		params.TrialStart,
		params.TrialEnd != nil,
		params.TrialEnd,
		params.CancelAtPeriodEnd,
		params.CanceledAt != nil,
		params.CanceledAt,
		params.SuspendedAt != nil,
		params.SuspendedAt,
		optionalMap(params.Metadata),
	).Scan(subscriptionScanDest(&subscription, &metadataBytes)...)
	if err != nil {
		return model.Subscription{}, err
	}
	if err := decodeMap(metadataBytes, &subscription.Metadata); err != nil {
		return model.Subscription{}, err
	}
	return subscription, nil
}

func (r *SubscriptionRepository) CreateEvent(ctx context.Context, params SubscriptionEventParams) (model.SubscriptionEvent, error) {
	metadata, err := encodeMap(params.Metadata)
	if err != nil {
		return model.SubscriptionEvent{}, err
	}
	var event model.SubscriptionEvent
	var metadataBytes []byte
	err = r.db.QueryRow(ctx, `
		INSERT INTO subscription_events (
			subscription_id,
			organization_id,
			event_type,
			old_status,
			new_status,
			actor_user_id,
			metadata
		)
		VALUES ($1::uuid, $2::uuid, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, '')::uuid, $7::jsonb)
		RETURNING
			id,
			subscription_id,
			organization_id,
			event_type,
			old_status,
			new_status,
			actor_user_id,
			metadata,
			created_at
	`,
		strings.TrimSpace(params.SubscriptionID),
		strings.TrimSpace(params.OrganizationID),
		canonicalKey(params.Type),
		subscriptionStatusValue(params.OldStatus),
		subscriptionStatusValue(params.NewStatus),
		stringPointerValue(params.ActorUserID),
		metadata,
	).Scan(
		&event.ID,
		&event.SubscriptionID,
		&event.OrganizationID,
		&event.Type,
		&event.OldStatus,
		&event.NewStatus,
		&event.ActorUserID,
		&metadataBytes,
		&event.CreatedAt,
	)
	if err != nil {
		return model.SubscriptionEvent{}, err
	}
	if err := decodeMap(metadataBytes, &event.Metadata); err != nil {
		return model.SubscriptionEvent{}, err
	}
	return event, nil
}

func subscriptionWhere(filter SubscriptionListFilter) (string, []any) {
	conditions := []string{}
	args := []any{}
	if filter.OrganizationID != "" {
		args = append(args, strings.TrimSpace(filter.OrganizationID))
		conditions = append(conditions, fmt.Sprintf("organization_id = $%d::uuid", len(args)))
	}
	if filter.PlanID != "" {
		args = append(args, strings.TrimSpace(filter.PlanID))
		conditions = append(conditions, fmt.Sprintf("plan_id = $%d::uuid", len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func subscriptionScanDest(subscription *model.Subscription, metadata *[]byte) []any {
	return []any{
		&subscription.ID,
		&subscription.OrganizationID,
		&subscription.PlanID,
		&subscription.Status,
		&subscription.BillingInterval,
		&subscription.CurrentPeriodStart,
		&subscription.CurrentPeriodEnd,
		&subscription.TrialStart,
		&subscription.TrialEnd,
		&subscription.CancelAtPeriodEnd,
		&subscription.CanceledAt,
		&subscription.SuspendedAt,
		metadata,
		&subscription.CreatedAt,
		&subscription.UpdatedAt,
	}
}

func subscriptionStatusValue(value *model.SubscriptionStatus) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func billingIntervalValue(value *model.BillingInterval) string {
	if value == nil {
		return ""
	}
	return string(*value)
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

func canonicalKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func stringPointerValue(value *string) any {
	if value == nil {
		return nil
	}
	return strings.TrimSpace(*value)
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
