package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database"
)

type SwitchOrganizationParams struct {
	UserID         string
	SessionID      string
	OrganizationID string
	RequestID      string
	IPAddress      string
	UserAgent      string
	SwitchedAt     time.Time
}

type SwitchRepository struct {
	db *database.Pool
}

func NewSwitchRepository(db *database.Pool) *SwitchRepository {
	return &SwitchRepository{db: db}
}

func (r *SwitchRepository) ListActiveByUser(
	ctx context.Context,
	userID string,
) ([]MembershipOrganization, error) {
	results, _, err := NewMembershipRepository(r.db).ListByUser(
		ctx,
		userID,
		MembershipListFilter{
			Status: model.MembershipStatusActive,
			Limit:  100,
		},
	)
	if err != nil {
		return nil, err
	}
	rolesByOrganization, err := listOrganizationRoles(ctx, r.db, userID, "")
	if err != nil {
		return nil, err
	}
	for index := range results {
		roles := rolesByOrganization[results[index].Organization.ID]
		results[index].RoleIDs = roles.IDs
		results[index].RoleSlugs = roles.Slugs
	}
	return results, nil
}

func (r *SwitchRepository) FindSessionSnapshot(
	ctx context.Context,
	userID string,
	sessionID string,
) (SessionOrganizationSnapshot, error) {
	return NewAuthenticatedResolverRepository(r.db).FindSessionSnapshot(
		ctx,
		userID,
		sessionID,
	)
}

func (r *SwitchRepository) Switch(
	ctx context.Context,
	params SwitchOrganizationParams,
) (MembershipOrganization, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return MembershipOrganization{}, err
	}
	defer tx.Rollback(ctx)

	var previousOrganizationID sql.NullString
	err = tx.QueryRow(ctx, `
		SELECT active_organization_id::text
		FROM sessions
		WHERE id = $1::uuid
			AND user_id = $2::uuid
			AND revoked_at IS NULL
			AND expires_at > $3
		FOR UPDATE
	`, strings.TrimSpace(params.SessionID), strings.TrimSpace(params.UserID),
		params.SwitchedAt).Scan(&previousOrganizationID)
	if err != nil {
		return MembershipOrganization{}, err
	}

	var result MembershipOrganization
	var metadataBytes []byte
	dest := membershipScanDest(&result.Membership)
	dest = append(dest, organizationScanDest(&result.Organization, &metadataBytes)...)
	err = tx.QueryRow(ctx, `
		SELECT
			`+prefixedMembershipSelectColumns("membership")+`,
			`+prefixedOrganizationSelectColumns("organization")+`
		FROM organization_memberships membership
		JOIN organizations organization ON organization.id = membership.organization_id
		WHERE membership.user_id = $1::uuid
			AND membership.organization_id = $2::uuid
			AND membership.status = 'active'
			AND membership.removed_at IS NULL
			AND organization.status = 'active'
			AND organization.deleted_at IS NULL
		LIMIT 1
	`, strings.TrimSpace(params.UserID), strings.TrimSpace(params.OrganizationID)).
		Scan(dest...)
	if err != nil {
		return MembershipOrganization{}, err
	}
	if err := decodeMetadata(metadataBytes, &result.Organization.Metadata); err != nil {
		return MembershipOrganization{}, err
	}
	rolesByOrganization, err := listOrganizationRoles(
		ctx,
		tx,
		params.UserID,
		result.Organization.ID,
	)
	if err != nil {
		return MembershipOrganization{}, err
	}
	roles := rolesByOrganization[result.Organization.ID]
	result.RoleIDs = roles.IDs
	result.RoleSlugs = roles.Slugs

	commandTag, err := tx.Exec(ctx, `
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
	`, strings.TrimSpace(params.SessionID), strings.TrimSpace(params.UserID),
		result.Organization.ID, result.Membership.ID, result.Membership.Version,
		params.SwitchedAt)
	if err != nil {
		return MembershipOrganization{}, err
	}
	if commandTag.RowsAffected() == 0 {
		return MembershipOrganization{}, pgx.ErrNoRows
	}

	auditMetadata, err := json.Marshal(map[string]any{
		"previous_organization_id": previousOrganizationID.String,
		"organization_id":          result.Organization.ID,
		"membership_version":       result.Membership.Version,
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
	`, result.Organization.ID, result.Membership.ID, strings.TrimSpace(params.SessionID),
		strings.TrimSpace(params.UserID), string(auditMetadata),
		strings.TrimSpace(params.IPAddress), strings.TrimSpace(params.UserAgent),
		strings.TrimSpace(params.RequestID), params.SwitchedAt); err != nil {
		return MembershipOrganization{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return MembershipOrganization{}, err
	}
	return result, nil
}

type organizationRoleQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type organizationRoles struct {
	IDs   []string
	Slugs []string
}

func listOrganizationRoles(
	ctx context.Context,
	querier organizationRoleQuerier,
	userID string,
	organizationID string,
) (map[string]organizationRoles, error) {
	rows, err := querier.Query(ctx, `
		SELECT
			user_role.organization_id::text,
			user_role.role_id::text,
			role.slug
		FROM user_roles user_role
		JOIN roles role ON role.id = user_role.role_id
		WHERE user_role.user_id = $1::uuid
			AND user_role.organization_id IS NOT NULL
			AND (
				NULLIF($2, '')::uuid IS NULL
				OR user_role.organization_id = NULLIF($2, '')::uuid
			)
		ORDER BY user_role.organization_id, role.slug, user_role.role_id
	`, strings.TrimSpace(userID), strings.TrimSpace(organizationID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[string]organizationRoles)
	for rows.Next() {
		var currentOrganizationID string
		var roleID string
		var roleSlug string
		if err := rows.Scan(&currentOrganizationID, &roleID, &roleSlug); err != nil {
			return nil, err
		}
		roles := results[currentOrganizationID]
		roles.IDs = append(roles.IDs, roleID)
		roles.Slugs = append(roles.Slugs, roleSlug)
		results[currentOrganizationID] = roles
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
