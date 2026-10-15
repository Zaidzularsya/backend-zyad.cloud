package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
)

type memberRepository struct {
	db *database.Pool
}

func NewMemberRepository(db *database.Pool) MemberRepository {
	return &memberRepository{db: db}
}

func (r *memberRepository) ListActive(ctx context.Context, scope coretenant.Scope) ([]domain.OrganizationMember, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT u.id, u.name, u.email
		FROM organization_memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.organization_id = $1
			AND m.status = 'active'
			AND u.deleted_at IS NULL
		ORDER BY lower(u.name)
	`

	rows, err := r.db.Query(ctx, query, scope.OrganizationID())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := []domain.OrganizationMember{}
	for rows.Next() {
		var member domain.OrganizationMember
		if err := rows.Scan(&member.UserID, &member.Name, &member.Email); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (r *memberRepository) IsActiveMember(ctx context.Context, scope coretenant.Scope, userID string) (bool, error) {
	if !scope.IsValid() {
		return false, coretenant.ErrInvalidScope
	}

	query := `
		SELECT EXISTS (
			SELECT 1
			FROM organization_memberships m
			JOIN users u ON u.id = m.user_id
			WHERE m.organization_id = $1
				AND m.user_id::text = $2
				AND m.status = 'active'
				AND u.deleted_at IS NULL
		)
	`

	var exists bool
	if err := r.db.QueryRow(ctx, query, scope.OrganizationID(), userID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
