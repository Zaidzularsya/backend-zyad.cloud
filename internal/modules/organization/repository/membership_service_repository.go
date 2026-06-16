package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database"
)

var ErrInvitationExpired = errors.New("membership invitation expired")
var ErrMembershipAlreadyExists = errors.New("membership already exists")
var ErrMembershipStatusInvalid = errors.New("membership status transition is invalid")
var ErrRoleNotFound = errors.New("one or more roles were not found")

type MembershipServiceRepository struct {
	db *database.Pool
}

type InviteMembershipParams struct {
	OrganizationID      string
	Email               string
	RoleIDs             []string
	InvitationTokenHash string
	InvitationExpiresAt time.Time
	ActorUserID         string
	InvitedAt           time.Time
}

type AcceptMembershipParams struct {
	UserID              string
	InvitationTokenHash string
	AcceptedAt          time.Time
}

type ChangeMembershipStatusParams struct {
	OrganizationID string
	MembershipID   string
	Status         model.MembershipStatus
	Reason         string
	ActorUserID    string
	ChangedAt      time.Time
}

type SyncMembershipRolesParams struct {
	OrganizationID string
	MembershipID   string
	RoleIDs        []string
	ActorUserID    string
	ChangedAt      time.Time
}

type TransferMembershipOwnershipParams struct {
	OrganizationID   string
	FromMembershipID string
	ToMembershipID   string
	ActorUserID      string
	ChangedAt        time.Time
}

type MembershipDetail struct {
	Membership model.Membership
	UserName   string
	UserEmail  string
	RoleIDs    []string
	RoleSlugs  []string
}

func NewMembershipServiceRepository(db *database.Pool) *MembershipServiceRepository {
	return &MembershipServiceRepository{db: db}
}

func (r *MembershipServiceRepository) ListByOrganization(
	ctx context.Context,
	organizationID string,
	filter MembershipListFilter,
) ([]model.Membership, int64, error) {
	return NewMembershipRepository(r.db).ListByOrganization(ctx, organizationID, filter)
}

func (r *MembershipServiceRepository) ListDetailsByOrganization(
	ctx context.Context,
	organizationID string,
	filter MembershipListFilter,
) ([]MembershipDetail, int64, error) {
	where, args := membershipWhere(filter)
	args = append([]any{strings.TrimSpace(organizationID)}, args...)
	where = " WHERE organization_id = $1::uuid" + renumberMembershipWhere(where, 1)

	var total int64
	if err := r.db.QueryRow(ctx, `
		SELECT count(*)
		FROM organization_memberships`+where,
		args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := membershipPagination(filter)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT
			membership.*,
			user_account.name,
			user_account.email,
			COALESCE(role_summary.role_ids, ARRAY[]::text[]),
			COALESCE(role_summary.role_slugs, ARRAY[]::text[])
		FROM (
			SELECT `+membershipSelectColumns+`
			FROM organization_memberships
			`+where+`
			ORDER BY is_owner DESC, created_at ASC, id ASC
			LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args))+`
		) membership
		JOIN users user_account ON user_account.id = membership.user_id
		LEFT JOIN LATERAL (
			SELECT
				array_agg(role.id::text ORDER BY role.slug) AS role_ids,
				array_agg(role.slug::text ORDER BY role.slug) AS role_slugs
			FROM user_roles user_role
			JOIN roles role ON role.id = user_role.role_id
			WHERE user_role.user_id = membership.user_id
				AND user_role.organization_id = membership.organization_id
		) role_summary ON true
		ORDER BY membership.is_owner DESC, membership.created_at ASC, membership.id ASC
	`,
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	details := make([]MembershipDetail, 0)
	for rows.Next() {
		var detail MembershipDetail
		if err := rows.Scan(
			append(
				membershipScanDest(&detail.Membership),
				&detail.UserName,
				&detail.UserEmail,
				&detail.RoleIDs,
				&detail.RoleSlugs,
			)...,
		); err != nil {
			return nil, 0, err
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return details, total, nil
}

func (r *MembershipServiceRepository) Invite(
	ctx context.Context,
	params InviteMembershipParams,
) (model.Membership, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.Membership{}, err
	}
	defer tx.Rollback(ctx)

	if err := lockOrganization(ctx, tx, params.OrganizationID); err != nil {
		return model.Membership{}, err
	}

	var userID string
	err = tx.QueryRow(ctx, `
		SELECT id
		FROM users
		WHERE lower(email) = lower($1)
			AND deleted_at IS NULL
			AND status IN ('active', 'pending', 'invited')
		FOR UPDATE
	`, strings.TrimSpace(params.Email)).Scan(&userID)
	if err != nil {
		return model.Membership{}, err
	}
	if err := validateRoleIDs(ctx, tx, params.RoleIDs); err != nil {
		return model.Membership{}, err
	}

	var membership model.Membership
	err = tx.QueryRow(ctx, `
		SELECT `+membershipSelectColumns+`
		FROM organization_memberships
		WHERE organization_id = $1::uuid
			AND user_id = $2::uuid
		FOR UPDATE
	`, strings.TrimSpace(params.OrganizationID), userID).
		Scan(membershipScanDest(&membership)...)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		err = tx.QueryRow(ctx, `
			INSERT INTO organization_memberships (
				organization_id,
				user_id,
				status,
				is_owner,
				invited_by,
				invited_email,
				invitation_token_hash,
				invitation_expires_at,
				invited_at
			)
			VALUES (
				$1::uuid, $2::uuid, 'invited', false, NULLIF($3, '')::uuid,
				$4, $5, $6, $7
			)
			RETURNING `+membershipSelectColumns,
			strings.TrimSpace(params.OrganizationID),
			userID,
			strings.TrimSpace(params.ActorUserID),
			strings.ToLower(strings.TrimSpace(params.Email)),
			params.InvitationTokenHash,
			params.InvitationExpiresAt,
			params.InvitedAt,
		).Scan(membershipScanDest(&membership)...)
	case err != nil:
		return model.Membership{}, err
	case membership.Status == model.MembershipStatusActive ||
		membership.Status == model.MembershipStatusSuspended:
		return model.Membership{}, ErrMembershipAlreadyExists
	default:
		err = tx.QueryRow(ctx, `
			UPDATE organization_memberships
			SET
				status = 'invited',
				is_owner = false,
				invited_by = NULLIF($2, '')::uuid,
				invited_email = $3,
				invitation_token_hash = $4,
				invitation_expires_at = $5,
				invited_at = $6,
				accepted_at = NULL,
				suspended_at = NULL,
				removed_at = NULL,
				updated_at = $6
			WHERE id = $1::uuid
			RETURNING `+membershipSelectColumns,
			membership.ID,
			strings.TrimSpace(params.ActorUserID),
			strings.ToLower(strings.TrimSpace(params.Email)),
			params.InvitationTokenHash,
			params.InvitationExpiresAt,
			params.InvitedAt,
		).Scan(membershipScanDest(&membership)...)
	}
	if err != nil {
		return model.Membership{}, err
	}
	if _, err := replaceOrganizationRoles(
		ctx,
		tx,
		membership,
		params.RoleIDs,
		params.ActorUserID,
		params.InvitedAt,
	); err != nil {
		return model.Membership{}, err
	}
	if err := reloadMembership(ctx, tx, &membership); err != nil {
		return model.Membership{}, err
	}
	if err := insertMembershipAudit(
		ctx, tx, membership, params.ActorUserID, "member_invited",
		map[string]any{"role_count": len(params.RoleIDs)}, params.InvitedAt,
	); err != nil {
		return model.Membership{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Membership{}, err
	}
	return membership, nil
}

func (r *MembershipServiceRepository) Accept(
	ctx context.Context,
	params AcceptMembershipParams,
) (model.Membership, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.Membership{}, err
	}
	defer tx.Rollback(ctx)

	var organizationID string
	err = tx.QueryRow(ctx, `
		SELECT organization_id
		FROM organization_memberships
		WHERE invitation_token_hash = $1
			AND user_id = $2::uuid
			AND status = 'invited'
			AND removed_at IS NULL
		ORDER BY invited_at DESC NULLS LAST
		LIMIT 1
	`, params.InvitationTokenHash, strings.TrimSpace(params.UserID)).Scan(&organizationID)
	if err != nil {
		return model.Membership{}, err
	}
	if err := lockOrganization(ctx, tx, organizationID); err != nil {
		return model.Membership{}, err
	}

	var membership model.Membership
	err = tx.QueryRow(ctx, `
		SELECT `+membershipSelectColumns+`
		FROM organization_memberships
		WHERE invitation_token_hash = $1
			AND user_id = $2::uuid
			AND status = 'invited'
			AND removed_at IS NULL
		ORDER BY invited_at DESC NULLS LAST
		LIMIT 1
		FOR UPDATE
	`, params.InvitationTokenHash, strings.TrimSpace(params.UserID)).
		Scan(membershipScanDest(&membership)...)
	if err != nil {
		return model.Membership{}, err
	}
	if membership.InvitationExpiresAt == nil ||
		!membership.InvitationExpiresAt.After(params.AcceptedAt) {
		return model.Membership{}, ErrInvitationExpired
	}
	err = tx.QueryRow(ctx, `
		UPDATE organization_memberships
		SET
			status = 'active',
			invitation_token_hash = NULL,
			invitation_expires_at = NULL,
			accepted_at = $2,
			suspended_at = NULL,
			removed_at = NULL,
			updated_at = $2
		WHERE id = $1::uuid
		RETURNING `+membershipSelectColumns,
		membership.ID,
		params.AcceptedAt,
	).Scan(membershipScanDest(&membership)...)
	if err != nil {
		return model.Membership{}, err
	}
	if err := insertMembershipAudit(
		ctx, tx, membership, params.UserID, "member_invitation_accepted",
		map[string]any{}, params.AcceptedAt,
	); err != nil {
		return model.Membership{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Membership{}, err
	}
	return membership, nil
}

func (r *MembershipServiceRepository) ChangeStatus(
	ctx context.Context,
	params ChangeMembershipStatusParams,
) (model.Membership, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.Membership{}, err
	}
	defer tx.Rollback(ctx)

	if err := lockOrganization(ctx, tx, params.OrganizationID); err != nil {
		return model.Membership{}, err
	}
	membership, err := lockMembership(ctx, tx, params.MembershipID)
	if err != nil {
		return model.Membership{}, err
	}
	if membership.OrganizationID != strings.TrimSpace(params.OrganizationID) {
		return model.Membership{}, pgx.ErrNoRows
	}
	if !validMembershipTransition(membership.Status, params.Status) {
		return model.Membership{}, ErrMembershipStatusInvalid
	}
	if membership.IsOwner && membership.IsActive() &&
		params.Status != model.MembershipStatusActive {
		if err := ensureAnotherActiveOwner(ctx, tx, membership); err != nil {
			return model.Membership{}, err
		}
	}
	if params.Status == model.MembershipStatusRemoved {
		if _, err := tx.Exec(ctx, `
			DELETE FROM user_roles
			WHERE user_id = $1::uuid
				AND organization_id = $2::uuid
		`, membership.UserID, membership.OrganizationID); err != nil {
			return model.Membership{}, err
		}
	}

	previousStatus := membership.Status
	err = tx.QueryRow(ctx, `
		UPDATE organization_memberships
		SET
			status = $2::varchar,
			accepted_at = CASE
				WHEN $2::varchar = 'active' THEN COALESCE(accepted_at, $3)
				ELSE accepted_at
			END,
			suspended_at = CASE
				WHEN $2::varchar = 'suspended' THEN $3
				WHEN $2::varchar = 'active' THEN NULL
				ELSE suspended_at
			END,
			removed_at = CASE WHEN $2::varchar = 'removed' THEN $3 ELSE NULL END,
			invitation_token_hash = CASE
				WHEN $2::varchar = 'removed' THEN NULL
				ELSE invitation_token_hash
			END,
			invitation_expires_at = CASE
				WHEN $2::varchar = 'removed' THEN NULL
				ELSE invitation_expires_at
			END,
			updated_at = $3
		WHERE id = $1::uuid
		RETURNING `+membershipSelectColumns,
		membership.ID,
		string(params.Status),
		params.ChangedAt,
	).Scan(membershipScanDest(&membership)...)
	if err != nil {
		return model.Membership{}, err
	}
	if params.Status == model.MembershipStatusSuspended ||
		params.Status == model.MembershipStatusRemoved {
		if err := revokeMembershipSessions(ctx, tx, membership.ID, params.ChangedAt); err != nil {
			return model.Membership{}, err
		}
	}
	if err := insertMembershipAudit(
		ctx, tx, membership, params.ActorUserID, "member_status_changed",
		map[string]any{
			"from_status": previousStatus,
			"to_status":   params.Status,
			"reason":      strings.TrimSpace(params.Reason),
		},
		params.ChangedAt,
	); err != nil {
		return model.Membership{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Membership{}, err
	}
	return membership, nil
}

func (r *MembershipServiceRepository) SyncRoles(
	ctx context.Context,
	params SyncMembershipRolesParams,
) (model.Membership, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.Membership{}, err
	}
	defer tx.Rollback(ctx)

	if err := lockOrganization(ctx, tx, params.OrganizationID); err != nil {
		return model.Membership{}, err
	}
	membership, err := lockMembership(ctx, tx, params.MembershipID)
	if err != nil {
		return model.Membership{}, err
	}
	if membership.OrganizationID != strings.TrimSpace(params.OrganizationID) {
		return model.Membership{}, pgx.ErrNoRows
	}
	if err := validateRoleIDs(ctx, tx, params.RoleIDs); err != nil {
		return model.Membership{}, err
	}
	changed, err := replaceOrganizationRoles(
		ctx, tx, membership, params.RoleIDs, params.ActorUserID, params.ChangedAt,
	)
	if err != nil {
		return model.Membership{}, err
	}
	if changed {
		if err := revokeMembershipSessions(ctx, tx, membership.ID, params.ChangedAt); err != nil {
			return model.Membership{}, err
		}
		if err := reloadMembership(ctx, tx, &membership); err != nil {
			return model.Membership{}, err
		}
		if err := insertMembershipAudit(
			ctx, tx, membership, params.ActorUserID, "member_roles_changed",
			map[string]any{"role_count": len(params.RoleIDs)}, params.ChangedAt,
		); err != nil {
			return model.Membership{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Membership{}, err
	}
	return membership, nil
}

func (r *MembershipServiceRepository) TransferOwnership(
	ctx context.Context,
	params TransferMembershipOwnershipParams,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := lockOrganization(ctx, tx, params.OrganizationID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
		SELECT `+membershipSelectColumns+`
		FROM organization_memberships
		WHERE organization_id = $1::uuid
			AND id = ANY($2::uuid[])
		ORDER BY id
		FOR UPDATE
	`, strings.TrimSpace(params.OrganizationID), []string{
		strings.TrimSpace(params.FromMembershipID),
		strings.TrimSpace(params.ToMembershipID),
	})
	if err != nil {
		return err
	}
	defer rows.Close()

	memberships := make(map[string]model.Membership, 2)
	for rows.Next() {
		var membership model.Membership
		if err := rows.Scan(membershipScanDest(&membership)...); err != nil {
			return err
		}
		memberships[membership.ID] = membership
	}
	if err := rows.Err(); err != nil {
		return err
	}
	from := memberships[strings.TrimSpace(params.FromMembershipID)]
	to := memberships[strings.TrimSpace(params.ToMembershipID)]
	if !from.IsOwner || !from.IsActive() || !to.IsActive() || from.ID == to.ID {
		return pgx.ErrNoRows
	}
	if _, err := tx.Exec(ctx, `
		UPDATE organization_memberships
		SET
			is_owner = CASE WHEN id = $2::uuid THEN false ELSE true END,
			updated_at = $4
		WHERE organization_id = $1::uuid
			AND id = ANY($3::uuid[])
	`, strings.TrimSpace(params.OrganizationID), from.ID, []string{from.ID, to.ID},
		params.ChangedAt); err != nil {
		return err
	}
	if err := revokeMembershipSessions(ctx, tx, from.ID, params.ChangedAt); err != nil {
		return err
	}
	if err := revokeMembershipSessions(ctx, tx, to.ID, params.ChangedAt); err != nil {
		return err
	}
	to.IsOwner = true
	if err := insertMembershipAudit(
		ctx, tx, to, params.ActorUserID, "membership_ownership_transferred",
		map[string]any{"from_membership_id": from.ID, "to_membership_id": to.ID},
		params.ChangedAt,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validateRoleIDs(ctx context.Context, tx pgx.Tx, roleIDs []string) error {
	if len(roleIDs) == 0 {
		return nil
	}
	var count int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM roles
		WHERE id = ANY($1::uuid[])
	`, roleIDs).Scan(&count); err != nil {
		return err
	}
	if count != len(roleIDs) {
		return ErrRoleNotFound
	}
	return nil
}

func replaceOrganizationRoles(
	ctx context.Context,
	tx pgx.Tx,
	membership model.Membership,
	roleIDs []string,
	actorUserID string,
	changedAt time.Time,
) (bool, error) {
	tag, err := tx.Exec(ctx, `
		DELETE FROM user_roles
		WHERE user_id = $1::uuid
			AND organization_id = $2::uuid
			AND NOT (role_id = ANY($3::uuid[]))
	`, membership.UserID, membership.OrganizationID, roleIDs)
	if err != nil {
		return false, err
	}
	changed := tag.RowsAffected() > 0
	for _, roleID := range roleIDs {
		tag, err = tx.Exec(ctx, `
			INSERT INTO user_roles (
				id, user_id, role_id, organization_id, assigned_by, assigned_at
			)
			VALUES (
				gen_random_uuid(), $1::uuid, $2::uuid, $3::uuid,
				NULLIF($4, '')::uuid, $5
			)
			ON CONFLICT (user_id, role_id, organization_id)
				WHERE organization_id IS NOT NULL
			DO NOTHING
		`, membership.UserID, roleID, membership.OrganizationID,
			strings.TrimSpace(actorUserID), changedAt)
		if err != nil {
			return false, err
		}
		changed = changed || tag.RowsAffected() > 0
	}
	return changed, nil
}

func revokeMembershipSessions(
	ctx context.Context,
	tx pgx.Tx,
	membershipID string,
	revokedAt time.Time,
) error {
	if _, err := tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE session_id IN (
			SELECT id
			FROM sessions
			WHERE active_membership_id = $1::uuid
		)
	`, membershipID, revokedAt); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE active_membership_id = $1::uuid
	`, membershipID, revokedAt)
	return err
}

func reloadMembership(ctx context.Context, tx pgx.Tx, membership *model.Membership) error {
	return tx.QueryRow(ctx, `
		SELECT `+membershipSelectColumns+`
		FROM organization_memberships
		WHERE id = $1::uuid
	`, membership.ID).Scan(membershipScanDest(membership)...)
}

func insertMembershipAudit(
	ctx context.Context,
	tx pgx.Tx,
	membership model.Membership,
	actorUserID string,
	event string,
	metadata map[string]any,
	createdAt time.Time,
) error {
	return insertOrganizationAuditTx(
		ctx,
		tx,
		model.Organization{ID: membership.OrganizationID},
		membership,
		actorUserID,
		event,
		metadata,
		createdAt,
	)
}

func validMembershipTransition(from, to model.MembershipStatus) bool {
	switch from {
	case model.MembershipStatusActive:
		return to == model.MembershipStatusSuspended || to == model.MembershipStatusRemoved
	case model.MembershipStatusSuspended:
		return to == model.MembershipStatusActive || to == model.MembershipStatusRemoved
	default:
		return false
	}
}
