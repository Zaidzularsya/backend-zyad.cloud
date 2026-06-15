package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database"
)

var ErrOrganizationStatusStale = errors.New("organization status is stale")
var ErrPlatformOrganizationProtected = errors.New("platform organization is protected")

type LifecycleRepository struct {
	db *database.Pool
}

type CreateOrganizationBundleParams struct {
	Organization CreateOrganizationParams
	OwnerUserID  string
	ActorUserID  string
	Plan         []UpsertEntitlementParams
	CreatedAt    time.Time
}

type OrganizationBundle struct {
	Organization model.Organization
	Owner        model.Membership
	Entitlements []model.Entitlement
}

func NewLifecycleRepository(db *database.Pool) *LifecycleRepository {
	return &LifecycleRepository{db: db}
}

func (r *LifecycleRepository) CreateBundle(
	ctx context.Context,
	params CreateOrganizationBundleParams,
) (OrganizationBundle, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return OrganizationBundle{}, err
	}
	defer tx.Rollback(ctx)

	organizationParams := params.Organization
	organizationParams.Status = coretenant.OrganizationStatusProvisioning
	organization, err := createOrganizationTx(ctx, tx, organizationParams)
	if err != nil {
		return OrganizationBundle{}, err
	}

	owner, err := createOwnerMembershipTx(
		ctx,
		tx,
		organization.ID,
		params.OwnerUserID,
		params.ActorUserID,
		params.CreatedAt,
	)
	if err != nil {
		return OrganizationBundle{}, err
	}

	entitlements := make([]model.Entitlement, 0, len(params.Plan))
	for _, plan := range params.Plan {
		plan.OrganizationID = organization.ID
		plan.Source = model.EntitlementSourcePlan
		if plan.Status == "" {
			plan.Status = model.EntitlementStatusActive
		}
		if plan.EffectiveFrom.IsZero() {
			plan.EffectiveFrom = params.CreatedAt
		}
		entitlement, err := createEntitlementTx(ctx, tx, plan)
		if err != nil {
			return OrganizationBundle{}, err
		}
		entitlements = append(entitlements, entitlement)
	}

	if err := insertOrganizationAuditTx(
		ctx,
		tx,
		organization,
		owner,
		params.ActorUserID,
		"organization_created",
		map[string]any{
			"status":            organization.Status,
			"owner_user_id":     owner.UserID,
			"entitlement_count": len(entitlements),
		},
		params.CreatedAt,
	); err != nil {
		return OrganizationBundle{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return OrganizationBundle{}, err
	}
	return OrganizationBundle{
		Organization: organization,
		Owner:        owner,
		Entitlements: entitlements,
	}, nil
}

func (r *LifecycleRepository) ChangeStatus(
	ctx context.Context,
	organizationID string,
	expectedStatus coretenant.OrganizationStatus,
	status coretenant.OrganizationStatus,
	actorUserID string,
	reason string,
	changedAt time.Time,
) (model.Organization, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.Organization{}, err
	}
	defer tx.Rollback(ctx)

	var organization model.Organization
	var metadataBytes []byte
	err = tx.QueryRow(ctx, `
		SELECT `+organizationSelectColumns+`
		FROM organizations
		WHERE id = $1::uuid
			AND deleted_at IS NULL
			AND status <> 'archived'
		FOR UPDATE
	`, strings.TrimSpace(organizationID)).
		Scan(organizationScanDest(&organization, &metadataBytes)...)
	if err != nil {
		return model.Organization{}, err
	}
	if err := decodeMetadata(metadataBytes, &organization.Metadata); err != nil {
		return model.Organization{}, err
	}
	if organization.Status != expectedStatus {
		return model.Organization{}, ErrOrganizationStatusStale
	}
	if organization.IsPlatform() &&
		(status == coretenant.OrganizationStatusDisabled ||
			status == coretenant.OrganizationStatusArchived) {
		return model.Organization{}, ErrPlatformOrganizationProtected
	}

	previousStatus := organization.Status
	deletedAt := any(nil)
	if status == coretenant.OrganizationStatusArchived {
		deletedAt = changedAt
	}
	err = tx.QueryRow(ctx, `
		UPDATE organizations
		SET status = $2, deleted_at = $3, updated_at = $4
		WHERE id = $1::uuid
		RETURNING `+organizationSelectColumns,
		organization.ID,
		string(status),
		deletedAt,
		changedAt,
	).Scan(organizationScanDest(&organization, &metadataBytes)...)
	if err != nil {
		return model.Organization{}, err
	}
	if err := decodeMetadata(metadataBytes, &organization.Metadata); err != nil {
		return model.Organization{}, err
	}

	if status == coretenant.OrganizationStatusSuspended ||
		status == coretenant.OrganizationStatusDisabled ||
		status == coretenant.OrganizationStatusArchived {
		if _, err := tx.Exec(ctx, `
			UPDATE sessions
			SET revoked_at = COALESCE(revoked_at, $2)
			WHERE active_organization_id = $1::uuid
				AND revoked_at IS NULL
		`, organization.ID, changedAt); err != nil {
			return model.Organization{}, err
		}
	}

	if err := insertOrganizationAuditTx(
		ctx,
		tx,
		organization,
		model.Membership{},
		actorUserID,
		"organization_status_changed",
		map[string]any{
			"from_status": previousStatus,
			"to_status":   status,
			"reason":      strings.TrimSpace(reason),
		},
		changedAt,
	); err != nil {
		return model.Organization{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Organization{}, err
	}
	return organization, nil
}

func createOrganizationTx(
	ctx context.Context,
	tx pgx.Tx,
	params CreateOrganizationParams,
) (model.Organization, error) {
	metadata, err := encodeMetadata(params.Metadata)
	if err != nil {
		return model.Organization{}, err
	}
	timezone := strings.TrimSpace(params.Timezone)
	if timezone == "" {
		timezone = "Asia/Jakarta"
	}
	locale := strings.TrimSpace(params.Locale)
	if locale == "" {
		locale = "id-ID"
	}
	dataPlacement := params.DataPlacement
	if dataPlacement == "" {
		dataPlacement = coretenant.DataPlacementShared
	}

	var organization model.Organization
	var metadataBytes []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO organizations (
			type, slug, name, status, timezone, locale, region, data_placement, metadata
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9::jsonb
		)
		RETURNING `+organizationSelectColumns,
		string(params.Type),
		canonicalSlug(params.Slug),
		strings.TrimSpace(params.Name),
		string(params.Status),
		timezone,
		locale,
		strings.TrimSpace(params.Region),
		string(dataPlacement),
		string(metadata),
	).Scan(organizationScanDest(&organization, &metadataBytes)...)
	if err != nil {
		return model.Organization{}, err
	}
	if err := decodeMetadata(metadataBytes, &organization.Metadata); err != nil {
		return model.Organization{}, err
	}
	return organization, nil
}

func createOwnerMembershipTx(
	ctx context.Context,
	tx pgx.Tx,
	organizationID string,
	ownerUserID string,
	actorUserID string,
	createdAt time.Time,
) (model.Membership, error) {
	var membership model.Membership
	err := tx.QueryRow(ctx, `
		INSERT INTO organization_memberships (
			organization_id,
			user_id,
			status,
			is_owner,
			invited_by,
			accepted_at,
			created_at,
			updated_at
		)
		VALUES (
			$1::uuid,
			$2::uuid,
			'active',
			true,
			NULLIF($3, '')::uuid,
			$4,
			$4,
			$4
		)
		RETURNING `+membershipSelectColumns,
		organizationID,
		strings.TrimSpace(ownerUserID),
		strings.TrimSpace(actorUserID),
		createdAt,
	).Scan(membershipScanDest(&membership)...)
	return membership, err
}

func createEntitlementTx(
	ctx context.Context,
	tx pgx.Tx,
	params UpsertEntitlementParams,
) (model.Entitlement, error) {
	limits, err := encodeLimits(params.Limits)
	if err != nil {
		return model.Entitlement{}, err
	}
	var entitlement model.Entitlement
	var limitsBytes []byte
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
		VALUES (
			$1::uuid,
			$2,
			'plan',
			NULLIF($3, ''),
			$4,
			$5::jsonb,
			$6,
			$7,
			NULLIF($8, ''),
			NULLIF($9, '')::uuid,
			NULLIF($9, '')::uuid
		)
		RETURNING `+entitlementSelectColumns,
		params.OrganizationID,
		canonicalFeatureKey(params.FeatureKey),
		strings.TrimSpace(params.SourceReference),
		string(params.Status),
		string(limits),
		params.EffectiveFrom,
		params.EffectiveUntil,
		strings.TrimSpace(params.Reason),
		strings.TrimSpace(params.ActorUserID),
	).Scan(entitlementScanDest(&entitlement, &limitsBytes)...)
	if err != nil {
		return model.Entitlement{}, err
	}
	if err := decodeLimits(limitsBytes, &entitlement.Limits); err != nil {
		return model.Entitlement{}, err
	}
	return entitlement, nil
}

func insertOrganizationAuditTx(
	ctx context.Context,
	tx pgx.Tx,
	organization model.Organization,
	membership model.Membership,
	actorUserID string,
	event string,
	metadata map[string]any,
	createdAt time.Time,
) error {
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	effectiveUserID := strings.TrimSpace(actorUserID)
	if membership.UserID != "" {
		effectiveUserID = membership.UserID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (
			module,
			event,
			organization_id,
			membership_id,
			actor_user_id,
			operator_user_id,
			effective_user_id,
			target_type,
			target_id,
			metadata,
			resolution_source,
			created_at
		)
		VALUES (
			'organization',
			$1,
			$2::uuid,
			NULLIF($3, '')::uuid,
			NULLIF($4, '')::uuid,
			NULLIF($4, '')::uuid,
			NULLIF($5, '')::uuid,
			'organization',
			$2::uuid,
			$6::jsonb,
			'internal',
			$7
		)
	`, event, organization.ID, membership.ID, strings.TrimSpace(actorUserID),
		effectiveUserID, string(metadataBytes), createdAt)
	return err
}
