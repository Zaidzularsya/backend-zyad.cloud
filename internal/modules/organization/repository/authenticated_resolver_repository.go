package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/platform/database"
)

type SessionOrganizationSnapshot struct {
	OrganizationID    string
	MembershipID      string
	MembershipVersion int64
}

type AuthenticatedResolverRepository struct {
	db *database.Pool
}

func NewAuthenticatedResolverRepository(db *database.Pool) *AuthenticatedResolverRepository {
	return &AuthenticatedResolverRepository{db: db}
}

func (r *AuthenticatedResolverRepository) FindSessionSnapshot(
	ctx context.Context,
	userID string,
	sessionID string,
) (SessionOrganizationSnapshot, error) {
	var organizationID sql.NullString
	var membershipID sql.NullString
	var membershipVersion sql.NullInt64
	err := r.db.QueryRow(ctx, `
		SELECT
			active_organization_id::text,
			active_membership_id::text,
			active_membership_version
		FROM sessions
		WHERE id = $1::uuid
			AND user_id = $2::uuid
			AND revoked_at IS NULL
			AND expires_at > now()
	`, strings.TrimSpace(sessionID), strings.TrimSpace(userID)).Scan(
		&organizationID,
		&membershipID,
		&membershipVersion,
	)
	if err != nil {
		return SessionOrganizationSnapshot{}, err
	}
	return SessionOrganizationSnapshot{
		OrganizationID:    organizationID.String,
		MembershipID:      membershipID.String,
		MembershipVersion: membershipVersion.Int64,
	}, nil
}

func (r *AuthenticatedResolverRepository) FindActiveMembershipOrganization(
	ctx context.Context,
	userID string,
	organizationID string,
) (MembershipOrganization, error) {
	return r.findActiveMembershipOrganization(
		ctx,
		userID,
		organizationID,
		"",
		0,
	)
}

func (r *AuthenticatedResolverRepository) FindSessionMembershipOrganization(
	ctx context.Context,
	userID string,
	snapshot SessionOrganizationSnapshot,
) (MembershipOrganization, error) {
	return r.findActiveMembershipOrganization(
		ctx,
		userID,
		snapshot.OrganizationID,
		snapshot.MembershipID,
		snapshot.MembershipVersion,
	)
}

func (r *AuthenticatedResolverRepository) ListActiveMembershipOrganizations(
	ctx context.Context,
	userID string,
	limit int,
) ([]MembershipOrganization, error) {
	membershipRepo := NewMembershipRepository(r.db)
	results, _, err := membershipRepo.ListByUser(ctx, userID, MembershipListFilter{
		Status: model.MembershipStatusActive,
		Limit:  limit,
	})
	return results, err
}

func (r *AuthenticatedResolverRepository) findActiveMembershipOrganization(
	ctx context.Context,
	userID string,
	organizationID string,
	membershipID string,
	membershipVersion int64,
) (MembershipOrganization, error) {
	var result MembershipOrganization
	var metadataBytes []byte
	query := `
		SELECT
			` + prefixedMembershipSelectColumns("membership") + `,
			` + prefixedOrganizationSelectColumns("organization") + `
		FROM organization_memberships membership
		JOIN organizations organization ON organization.id = membership.organization_id
		WHERE membership.user_id = $1::uuid
			AND membership.organization_id = $2::uuid
			AND membership.status = 'active'
			AND membership.removed_at IS NULL
			AND organization.status = 'active'
			AND organization.deleted_at IS NULL`
	args := []any{strings.TrimSpace(userID), strings.TrimSpace(organizationID)}
	if strings.TrimSpace(membershipID) != "" || membershipVersion > 0 {
		query += `
			AND membership.id = $3::uuid
			AND membership.version = $4`
		args = append(args, strings.TrimSpace(membershipID), membershipVersion)
	}
	query += " LIMIT 1"

	dest := membershipScanDest(&result.Membership)
	dest = append(dest, organizationScanDest(&result.Organization, &metadataBytes)...)
	err := r.db.QueryRow(ctx, query, args...).Scan(dest...)
	if err != nil {
		return MembershipOrganization{}, err
	}
	if err := decodeMetadata(metadataBytes, &result.Organization.Metadata); err != nil {
		return MembershipOrganization{}, err
	}
	return result, nil
}

func IsResolverNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
