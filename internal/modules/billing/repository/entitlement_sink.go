package repository

import (
	"context"
	"strings"
	"time"

	billingmodel "zyad.cloud/internal/modules/billing/model"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/platform/database"
)

type EntitlementSink struct {
	store *organizationrepo.EntitlementRepository
	db    *database.Pool
}

type SyncPlanEntitlementsParams struct {
	OrganizationID string
	SubscriptionID string
	Entitlements   []billingmodel.PlanEntitlement
	EffectiveFrom  time.Time
	EffectiveUntil *time.Time
	ActorUserID    string
	Reason         string
}

func NewEntitlementSink(db *database.Pool) *EntitlementSink {
	return &EntitlementSink{
		store: organizationrepo.NewEntitlementRepository(db),
		db:    db,
	}
}

func (s *EntitlementSink) SyncPlanEntitlements(
	ctx context.Context,
	params SyncPlanEntitlementsParams,
) ([]organizationmodel.Entitlement, error) {
	if params.Reason == "" {
		params.Reason = "billing plan entitlement sync"
	}
	entitlements := make([]organizationmodel.Entitlement, 0, len(params.Entitlements))
	for _, entitlement := range params.Entitlements {
		synced, err := s.store.Upsert(ctx, organizationrepo.UpsertEntitlementParams{
			OrganizationID:  params.OrganizationID,
			FeatureKey:      entitlement.FeatureKey,
			Source:          organizationmodel.EntitlementSourcePlan,
			SourceReference: params.SubscriptionID,
			Status:          organizationmodel.EntitlementStatusActive,
			Limits:          runtimeLimits(entitlement),
			EffectiveFrom:   params.EffectiveFrom,
			EffectiveUntil:  params.EffectiveUntil,
			Reason:          params.Reason,
			ActorUserID:     params.ActorUserID,
		})
		if err != nil {
			return nil, err
		}
		entitlements = append(entitlements, synced)
	}
	return entitlements, nil
}

func (s *EntitlementSink) ExpirePlanEntitlements(
	ctx context.Context,
	organizationID string,
	subscriptionID string,
	effectiveUntil time.Time,
	actorUserID string,
	reason string,
) (int64, error) {
	if strings.TrimSpace(reason) == "" {
		reason = "billing plan entitlement expired"
	}
	commandTag, err := s.db.Exec(ctx, `
		UPDATE organization_entitlements
		SET
			status = 'expired',
			effective_until = $3,
			reason = NULLIF($5, ''),
			updated_by = NULLIF($4, '')::uuid,
			updated_at = now()
		WHERE organization_id = $1::uuid
			AND source = 'plan'
			AND source_reference = $2
			AND status = 'active'
	`, strings.TrimSpace(organizationID), strings.TrimSpace(subscriptionID), effectiveUntil, strings.TrimSpace(actorUserID), strings.TrimSpace(reason))
	if err != nil {
		return 0, err
	}
	return commandTag.RowsAffected(), nil
}

func runtimeLimits(entitlement billingmodel.PlanEntitlement) map[string]any {
	limits := map[string]any{}
	for key, value := range entitlement.Limits {
		limits[key] = value
	}
	switch {
	case entitlement.ValueBool != nil:
		limits["value"] = *entitlement.ValueBool
		limits["enabled"] = *entitlement.ValueBool
	case entitlement.ValueInt != nil:
		limits["value"] = *entitlement.ValueInt
		limits["limit"] = *entitlement.ValueInt
	case entitlement.ValueDecimal != nil:
		limits["value"] = strings.TrimSpace(*entitlement.ValueDecimal)
	case entitlement.ValueString != nil:
		limits["value"] = strings.TrimSpace(*entitlement.ValueString)
	}
	return limits
}
