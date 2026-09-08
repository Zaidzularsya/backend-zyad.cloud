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

var ErrLastActiveOwner = errors.New("organization must retain at least one active owner")

const membershipSelectColumns = `
	id,
	organization_id,
	user_id,
	status,
	is_owner,
	version,
	COALESCE(invited_by::text, ''),
	COALESCE(invited_email, ''),
	COALESCE(invitation_token_hash, ''),
	invitation_expires_at,
	invited_at,
	accepted_at,
	suspended_at,
	removed_at,
	created_at,
	updated_at
`

type MembershipRepository struct {
	db *database.Pool
}

type CreateMembershipParams struct {
	OrganizationID      string
	UserID              string
	Status              model.MembershipStatus
	IsOwner             bool
	InvitedBy           string
	InvitedEmail        string
	InvitationTokenHash string
	InvitationExpiresAt *time.Time
	InvitedAt           *time.Time
	AcceptedAt          *time.Time
}

type MembershipListFilter struct {
	Status         model.MembershipStatus
	IncludeRemoved bool
	Limit          int
	Offset         int
}

type MembershipOrganization struct {
	Membership   model.Membership
	Organization model.Organization
	RoleIDs      []string
	RoleSlugs    []string
}

func NewMembershipRepository(db *database.Pool) *MembershipRepository {
	return &MembershipRepository{db: db}
}

func (r *MembershipRepository) Create(
	ctx context.Context,
	params CreateMembershipParams,
) (model.Membership, error) {
	status := params.Status
	if status == "" {
		status = model.MembershipStatusInvited
	}

	var membership model.Membership
	err := r.db.QueryRow(ctx, `
		INSERT INTO organization_memberships (
			organization_id,
			user_id,
			status,
			is_owner,
			invited_by,
			invited_email,
			invitation_token_hash,
			invitation_expires_at,
			invited_at,
			accepted_at
		)
		VALUES (
			$1::uuid,
			$2::uuid,
			$3,
			$4,
			NULLIF($5, '')::uuid,
			NULLIF($6, ''),
			NULLIF($7, ''),
			$8,
			$9,
			$10
		)
		RETURNING `+membershipSelectColumns,
		strings.TrimSpace(params.OrganizationID),
		strings.TrimSpace(params.UserID),
		string(status),
		params.IsOwner,
		strings.TrimSpace(params.InvitedBy),
		strings.TrimSpace(params.InvitedEmail),
		strings.TrimSpace(params.InvitationTokenHash),
		params.InvitationExpiresAt,
		params.InvitedAt,
		params.AcceptedAt,
	).Scan(membershipScanDest(&membership)...)
	if err != nil {
		return model.Membership{}, err
	}
	return membership, nil
}

func (r *MembershipRepository) FindByID(
	ctx context.Context,
	organizationID string,
	id string,
) (model.Membership, error) {
	var membership model.Membership
	err := r.db.QueryRow(ctx, `
		SELECT `+membershipSelectColumns+`
		FROM organization_memberships
		WHERE id = $1::uuid
			AND organization_id = $2::uuid
			AND status <> 'removed'
			AND removed_at IS NULL
	`, strings.TrimSpace(id), strings.TrimSpace(organizationID)).Scan(membershipScanDest(&membership)...)
	if err != nil {
		return model.Membership{}, err
	}
	return membership, nil
}

func (r *MembershipRepository) FindActive(
	ctx context.Context,
	userID string,
	organizationID string,
) (model.Membership, error) {
	return r.FindActiveVersion(ctx, userID, organizationID, 0)
}

func (r *MembershipRepository) FindActiveVersion(
	ctx context.Context,
	userID string,
	organizationID string,
	version int64,
) (model.Membership, error) {
	var membership model.Membership
	query := `
		SELECT ` + membershipSelectColumns + `
		FROM organization_memberships
		WHERE user_id = $1::uuid
			AND organization_id = $2::uuid
			AND status = 'active'
			AND removed_at IS NULL`
	args := []any{strings.TrimSpace(userID), strings.TrimSpace(organizationID)}
	if version > 0 {
		query += " AND version = $3"
		args = append(args, version)
	}

	err := r.db.QueryRow(ctx, query, args...).Scan(membershipScanDest(&membership)...)
	if err != nil {
		return model.Membership{}, err
	}
	return membership, nil
}

func (r *MembershipRepository) ListByOrganization(
	ctx context.Context,
	organizationID string,
	filter MembershipListFilter,
) ([]model.Membership, int64, error) {
	where, args := membershipWhere(filter)
	args = append([]any{strings.TrimSpace(organizationID)}, args...)
	where = " WHERE organization_id = $1::uuid" + renumberMembershipWhere(where, 1)

	var total int64
	if err := r.db.QueryRow(
		ctx,
		"SELECT count(*) FROM organization_memberships"+where,
		args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := membershipPagination(filter)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT `+membershipSelectColumns+`
		FROM organization_memberships`+where+`
		ORDER BY is_owner DESC, created_at ASC, id ASC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	memberships := make([]model.Membership, 0)
	for rows.Next() {
		var membership model.Membership
		if err := rows.Scan(membershipScanDest(&membership)...); err != nil {
			return nil, 0, err
		}
		memberships = append(memberships, membership)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return memberships, total, nil
}

func (r *MembershipRepository) ListByUser(
	ctx context.Context,
	userID string,
	filter MembershipListFilter,
) ([]MembershipOrganization, int64, error) {
	where, args := membershipWhere(filter)
	args = append([]any{strings.TrimSpace(userID)}, args...)
	where = " WHERE membership.user_id = $1::uuid" + renumberMembershipWhere(where, 1)
	where = qualifyMembershipWhere(where, "membership")

	var total int64
	if err := r.db.QueryRow(ctx, `
		SELECT count(*)
		FROM organization_memberships membership
		JOIN organizations organization ON organization.id = membership.organization_id`+where+`
			AND organization.deleted_at IS NULL
			AND organization.status <> 'archived'
	`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit, offset := membershipPagination(filter)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT
			`+prefixedMembershipSelectColumns("membership")+`,
			`+prefixedOrganizationSelectColumns("organization")+`
		FROM organization_memberships membership
		JOIN organizations organization ON organization.id = membership.organization_id`+where+`
			AND organization.deleted_at IS NULL
			AND organization.status <> 'archived'
		ORDER BY organization.name ASC, organization.id ASC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	results := make([]MembershipOrganization, 0)
	for rows.Next() {
		var result MembershipOrganization
		var metadataBytes []byte
		dest := membershipScanDest(&result.Membership)
		dest = append(dest, organizationScanDest(&result.Organization, &metadataBytes)...)
		if err := rows.Scan(dest...); err != nil {
			return nil, 0, err
		}
		if err := decodeMetadata(metadataBytes, &result.Organization.Metadata); err != nil {
			return nil, 0, err
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *MembershipRepository) UpdateStatus(
	ctx context.Context,
	organizationID string,
	id string,
	status model.MembershipStatus,
	changedAt time.Time,
) (model.Membership, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.Membership{}, err
	}
	defer tx.Rollback(ctx)

	if err := lockOrganization(ctx, tx, organizationID); err != nil {
		return model.Membership{}, err
	}
	membership, err := lockMembership(ctx, tx, id)
	if err != nil {
		return model.Membership{}, err
	}
	if membership.OrganizationID != strings.TrimSpace(organizationID) {
		return model.Membership{}, pgx.ErrNoRows
	}
	if membership.IsOwner && membership.IsActive() && status != model.MembershipStatusActive {
		if err := ensureAnotherActiveOwner(ctx, tx, membership); err != nil {
			return model.Membership{}, err
		}
	}

	var suspendedAt any
	var removedAt any
	switch status {
	case model.MembershipStatusActive:
	case model.MembershipStatusSuspended:
		suspendedAt = changedAt
	case model.MembershipStatusRemoved:
		removedAt = changedAt
	}

	err = tx.QueryRow(ctx, `
		UPDATE organization_memberships
		SET
			status = $2::varchar,
			accepted_at = CASE
				WHEN $2::varchar = 'active' THEN COALESCE(accepted_at, $3::timestamp)
				ELSE accepted_at
			END,
			suspended_at = CASE
				WHEN $2::varchar = 'suspended' THEN $4::timestamp
				WHEN $2::varchar = 'active' THEN NULL
				ELSE suspended_at
			END,
			removed_at = CASE
				WHEN $2::varchar = 'removed' THEN $5::timestamp
				WHEN $2::varchar <> 'removed' THEN NULL
				ELSE removed_at
			END,
			updated_at = $3::timestamp
		WHERE id = $1::uuid
		RETURNING `+membershipSelectColumns,
		strings.TrimSpace(id),
		string(status),
		changedAt,
		suspendedAt,
		removedAt,
	).Scan(membershipScanDest(&membership)...)
	if err != nil {
		return model.Membership{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Membership{}, err
	}
	return membership, nil
}

func (r *MembershipRepository) SetOwner(
	ctx context.Context,
	organizationID string,
	id string,
	isOwner bool,
	changedAt time.Time,
) (model.Membership, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.Membership{}, err
	}
	defer tx.Rollback(ctx)

	if err := lockOrganization(ctx, tx, organizationID); err != nil {
		return model.Membership{}, err
	}
	membership, err := lockMembership(ctx, tx, id)
	if err != nil {
		return model.Membership{}, err
	}
	if membership.OrganizationID != strings.TrimSpace(organizationID) {
		return model.Membership{}, pgx.ErrNoRows
	}
	if membership.IsOwner && membership.IsActive() && !isOwner {
		if err := ensureAnotherActiveOwner(ctx, tx, membership); err != nil {
			return model.Membership{}, err
		}
	}

	err = tx.QueryRow(ctx, `
		UPDATE organization_memberships
		SET is_owner = $2, updated_at = $3
		WHERE id = $1::uuid
			AND status <> 'removed'
			AND removed_at IS NULL
		RETURNING `+membershipSelectColumns,
		strings.TrimSpace(id),
		isOwner,
		changedAt,
	).Scan(membershipScanDest(&membership)...)
	if err != nil {
		return model.Membership{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Membership{}, err
	}
	return membership, nil
}

func (r *MembershipRepository) Remove(
	ctx context.Context,
	organizationID string,
	id string,
	removedAt time.Time,
) (model.Membership, error) {
	return r.UpdateStatus(ctx, organizationID, id, model.MembershipStatusRemoved, removedAt)
}

func (r *MembershipRepository) TransferOwnership(
	ctx context.Context,
	organizationID string,
	fromMembershipID string,
	toMembershipID string,
	changedAt time.Time,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := lockOrganization(ctx, tx, organizationID); err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
		SELECT `+membershipSelectColumns+`
		FROM organization_memberships
		WHERE organization_id = $1::uuid
			AND id = ANY($2::uuid[])
		ORDER BY id
		FOR UPDATE
	`, strings.TrimSpace(organizationID), []string{
		strings.TrimSpace(fromMembershipID),
		strings.TrimSpace(toMembershipID),
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
	from, fromOK := memberships[strings.TrimSpace(fromMembershipID)]
	to, toOK := memberships[strings.TrimSpace(toMembershipID)]
	if !fromOK || !toOK || !from.IsOwner || !from.IsActive() || !to.IsActive() {
		return pgx.ErrNoRows
	}

	if _, err := tx.Exec(ctx, `
		UPDATE organization_memberships
		SET
			is_owner = CASE
				WHEN id = $2::uuid THEN false
				WHEN id = $3::uuid THEN true
			END,
			updated_at = $4
		WHERE organization_id = $1::uuid
			AND id = ANY($5::uuid[])
	`, strings.TrimSpace(organizationID), from.ID, to.ID, changedAt, []string{from.ID, to.ID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func membershipWhere(filter MembershipListFilter) (string, []any) {
	var query strings.Builder
	query.WriteString(" WHERE 1 = 1")
	args := make([]any, 0, 1)
	if !filter.IncludeRemoved {
		query.WriteString(" AND status <> 'removed' AND removed_at IS NULL")
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		query.WriteString(" AND status = $1")
	}
	return query.String(), args
}

func renumberMembershipWhere(where string, offset int) string {
	for index := 10; index >= 1; index-- {
		where = strings.ReplaceAll(
			where,
			fmt.Sprintf("$%d", index),
			fmt.Sprintf("$%d", index+offset),
		)
	}
	return strings.TrimPrefix(where, " WHERE 1 = 1")
}

func qualifyMembershipWhere(where string, alias string) string {
	where = strings.ReplaceAll(where, " status ", " "+alias+".status ")
	where = strings.ReplaceAll(where, " removed_at ", " "+alias+".removed_at ")
	return where
}

func membershipPagination(filter MembershipListFilter) (int, int) {
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

func membershipScanDest(membership *model.Membership) []any {
	return []any{
		&membership.ID,
		&membership.OrganizationID,
		&membership.UserID,
		&membership.Status,
		&membership.IsOwner,
		&membership.Version,
		&membership.InvitedBy,
		&membership.InvitedEmail,
		&membership.InvitationTokenHash,
		&membership.InvitationExpiresAt,
		&membership.InvitedAt,
		&membership.AcceptedAt,
		&membership.SuspendedAt,
		&membership.RemovedAt,
		&membership.CreatedAt,
		&membership.UpdatedAt,
	}
}

func lockMembership(ctx context.Context, tx pgx.Tx, id string) (model.Membership, error) {
	var membership model.Membership
	err := tx.QueryRow(ctx, `
		SELECT `+membershipSelectColumns+`
		FROM organization_memberships
		WHERE id = $1::uuid
			AND status <> 'removed'
			AND removed_at IS NULL
		FOR UPDATE
	`, strings.TrimSpace(id)).Scan(membershipScanDest(&membership)...)
	return membership, err
}

func lockOrganization(ctx context.Context, tx pgx.Tx, organizationID string) error {
	var id string
	return tx.QueryRow(ctx, `
		SELECT id
		FROM organizations
		WHERE id = $1::uuid
			AND deleted_at IS NULL
			AND status <> 'archived'
		FOR UPDATE
	`, strings.TrimSpace(organizationID)).Scan(&id)
}

func ensureAnotherActiveOwner(
	ctx context.Context,
	tx pgx.Tx,
	membership model.Membership,
) error {
	var ownerCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM organization_memberships
		WHERE organization_id = $1::uuid
			AND id <> $2::uuid
			AND is_owner = true
			AND status = 'active'
			AND removed_at IS NULL
	`, membership.OrganizationID, membership.ID).Scan(&ownerCount); err != nil {
		return err
	}
	if ownerCount == 0 {
		return ErrLastActiveOwner
	}
	return nil
}

func prefixedMembershipSelectColumns(alias string) string {
	return prefixSelectColumns(alias, membershipSelectColumns)
}

func prefixedOrganizationSelectColumns(alias string) string {
	return prefixSelectColumns(alias, organizationSelectColumns)
}

func prefixSelectColumns(alias string, columns string) string {
	lines := strings.Split(strings.TrimSpace(columns), "\n")
	for index, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, ",")
		if strings.HasPrefix(line, "COALESCE(") {
			line = strings.Replace(line, "COALESCE(", "COALESCE("+alias+".", 1)
		} else {
			line = alias + "." + line
		}
		if index < len(lines)-1 {
			line += ","
		}
		lines[index] = line
	}
	return strings.Join(lines, "\n")
}
