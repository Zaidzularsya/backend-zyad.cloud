package repository

import (
	"context"

	"zyad.cloud/internal/platform/database"
)

// Owner is a notification recipient for organization-level WhatsApp events.
type Owner struct {
	UserID string
	Name   string
	Email  string
}

// OwnerRepository reads organization owners (pattern:
// crm/repository/member_repository.go). organization_memberships, user_roles,
// and users are not under RLS, so the organization filter is explicit.
type OwnerRepository interface {
	// ListOrganizationOwners returns active members holding organization_owner
	// or super_admin in the organization, with a non-empty email.
	ListOrganizationOwners(ctx context.Context, organizationID string) ([]Owner, error)
}

type ownerRepository struct {
	db *database.Pool
}

func NewOwnerRepository(db *database.Pool) OwnerRepository {
	return &ownerRepository{db: db}
}

func (r *ownerRepository) ListOrganizationOwners(ctx context.Context, organizationID string) ([]Owner, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT u.id::text, u.name, u.email
		FROM organization_memberships m
		JOIN users u ON u.id = m.user_id
		-- Organization-scoped role, or a global (organization_id NULL) role
		-- such as super_admin for members of the platform organization.
		JOIN user_roles ur ON ur.user_id = u.id
			AND (ur.organization_id = m.organization_id OR ur.organization_id IS NULL)
		JOIN roles r ON r.id = ur.role_id
		WHERE m.organization_id = $1
			AND m.status = 'active'
			AND u.deleted_at IS NULL
			AND (r.slug IN ('organization_owner', 'super_admin') OR r.role_name IN ('organization_owner', 'super_admin'))
			AND COALESCE(u.email, '') <> ''
		ORDER BY u.email
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	owners := []Owner{}
	for rows.Next() {
		var owner Owner
		if err := rows.Scan(&owner.UserID, &owner.Name, &owner.Email); err != nil {
			return nil, err
		}
		owners = append(owners, owner)
	}
	return owners, rows.Err()
}
