package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database"
)

var ErrOrganizationOwnerRoleNotFound = errors.New("organization owner role not found")

type OnboardingRepository struct {
	db *database.Pool
}

type CreateWorkspaceParams struct {
	UserID        string
	SessionID     string
	Slug          string
	Name          string
	Timezone      string
	Locale        string
	Region        string
	Metadata      map[string]any
	RequestID     string
	IPAddress     string
	UserAgent     string
	CorrelationAt time.Time
}

func NewOnboardingRepository(db *database.Pool) *OnboardingRepository {
	return &OnboardingRepository{db: db}
}

func (r *OnboardingRepository) CreateWorkspace(
	ctx context.Context,
	params CreateWorkspaceParams,
) (MembershipOrganization, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return MembershipOrganization{}, err
	}
	defer tx.Rollback(ctx)

	var activeSessionID string
	var previousOrganizationID sql.NullString
	if err := tx.QueryRow(ctx, `
		SELECT id::text, active_organization_id::text
		FROM sessions
		WHERE id = $1::uuid
			AND user_id = $2::uuid
			AND revoked_at IS NULL
			AND expires_at > $3
		FOR UPDATE
	`, strings.TrimSpace(params.SessionID), strings.TrimSpace(params.UserID), params.CorrelationAt).
		Scan(&activeSessionID, &previousOrganizationID); err != nil {
		return MembershipOrganization{}, err
	}

	organization, err := createOrganizationTx(ctx, tx, CreateOrganizationParams{
		Type:          coretenant.OrganizationTypeCustomer,
		Slug:          params.Slug,
		Name:          params.Name,
		Status:        coretenant.OrganizationStatusActive,
		Timezone:      params.Timezone,
		Locale:        params.Locale,
		Region:        params.Region,
		DataPlacement: coretenant.DataPlacementShared,
		Metadata:      params.Metadata,
	})
	if err != nil {
		return MembershipOrganization{}, err
	}

	membership, err := createOwnerMembershipTx(
		ctx,
		tx,
		organization.ID,
		strings.TrimSpace(params.UserID),
		strings.TrimSpace(params.UserID),
		params.CorrelationAt,
	)
	if err != nil {
		return MembershipOrganization{}, err
	}

	roleID, err := findOrganizationOwnerRole(ctx, tx)
	if err != nil {
		return MembershipOrganization{}, err
	}
	if _, err := replaceOrganizationRoles(
		ctx,
		tx,
		membership,
		[]string{roleID},
		params.UserID,
		params.CorrelationAt,
	); err != nil {
		return MembershipOrganization{}, err
	}
	if err := reloadMembership(ctx, tx, &membership); err != nil {
		return MembershipOrganization{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sessions
		SET
			active_organization_id = $3::uuid,
			active_membership_id = $4::uuid,
			active_membership_version = $5,
			last_used_at = $6
		WHERE id = $1::uuid
			AND user_id = $2::uuid
			AND revoked_at IS NULL
			AND expires_at > $6
	`, activeSessionID, strings.TrimSpace(params.UserID), organization.ID, membership.ID,
		membership.Version, params.CorrelationAt); err != nil {
		return MembershipOrganization{}, err
	}

	if err := insertOrganizationAuditTx(
		ctx,
		tx,
		organization,
		membership,
		params.UserID,
		"workspace_onboarded",
		map[string]any{
			"previous_organization_id": previousOrganizationID.String,
			"role_slug":                "organization_owner",
		},
		params.CorrelationAt,
	); err != nil {
		return MembershipOrganization{}, err
	}

	auditMetadata, err := json.Marshal(map[string]any{
		"previous_organization_id": previousOrganizationID.String,
		"organization_id":          organization.ID,
		"membership_version":       membership.Version,
		"source":                   "onboarding",
	})
	if err != nil {
		return MembershipOrganization{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (
			module,
			event,
			organization_id,
			membership_id,
			session_id,
			actor_user_id,
			operator_user_id,
			effective_user_id,
			target_user_id,
			target_type,
			target_id,
			metadata,
			ip_address,
			user_agent,
			resolution_source,
			request_id,
			created_at
		)
		VALUES (
			'organization',
			'organization_switched',
			$1::uuid,
			$2::uuid,
			$3::uuid,
			$4::uuid,
			$4::uuid,
			$4::uuid,
			$4::uuid,
			'organization',
			$1::uuid,
			$5::jsonb,
			NULLIF($6, '')::inet,
			NULLIF($7, ''),
			'session',
			NULLIF($8, ''),
			$9
		)
	`, organization.ID, membership.ID, activeSessionID, strings.TrimSpace(params.UserID),
		string(auditMetadata), strings.TrimSpace(params.IPAddress),
		strings.TrimSpace(params.UserAgent), strings.TrimSpace(params.RequestID),
		params.CorrelationAt); err != nil {
		return MembershipOrganization{}, err
	}

	rolesByOrganization, err := listOrganizationRoles(ctx, tx, params.UserID, organization.ID)
	if err != nil {
		return MembershipOrganization{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return MembershipOrganization{}, err
	}

	roles := rolesByOrganization[organization.ID]
	return MembershipOrganization{
		Organization: organization,
		Membership:   membership,
		RoleIDs:      roles.IDs,
		RoleSlugs:    roles.Slugs,
	}, nil
}

func findOrganizationOwnerRole(ctx context.Context, tx pgx.Tx) (string, error) {
	var roleID string
	err := tx.QueryRow(ctx, `
		SELECT id::text
		FROM roles
		WHERE slug = 'organization_owner'
			OR role_name = 'organization_owner'
		ORDER BY created_at
		LIMIT 1
	`).Scan(&roleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrOrganizationOwnerRoleNotFound
	}
	return roleID, err
}
