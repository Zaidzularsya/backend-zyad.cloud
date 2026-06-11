package seeder

import (
	"context"

	"zyad.cloud/internal/platform/database"
)

type PermissionSeed struct {
	Name        string
	Description string
}

type RoleSeed struct {
	Name        string
	Description string
	Permissions []string
}

type SeedData struct {
	Permissions []PermissionSeed
	Roles       []RoleSeed
}

func DefaultSeedData() SeedData {
	return SeedData{
		Permissions: []PermissionSeed{
			{Name: "permission:read", Description: "Read permission data"},
			{Name: "permission:manage", Description: "Manage permission data"},
		},
		Roles: []RoleSeed{
			{
				Name:        "superadmin",
				Description: "Full platform administrator",
				Permissions: []string{
					"permission:read",
					"permission:manage",
				},
			},
		},
	}
}

func Seed(ctx context.Context, db *database.Pool, data SeedData) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, permission := range data.Permissions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO permissions (permission_name, description, created_at, updated_at)
			VALUES ($1, $2, now(), now())
			ON CONFLICT (permission_name)
			DO UPDATE SET description = EXCLUDED.description, updated_at = now()
		`, permission.Name, permission.Description); err != nil {
			return err
		}
	}

	for _, role := range data.Roles {
		var roleID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO roles (role_name, description, created_at, updated_at)
			VALUES ($1, $2, now(), now())
			ON CONFLICT (role_name)
			DO UPDATE SET description = EXCLUDED.description, updated_at = now()
			RETURNING id
		`, role.Name, role.Description).Scan(&roleID); err != nil {
			return err
		}

		for _, permissionName := range role.Permissions {
			var permissionID string
			if err := tx.QueryRow(ctx, `
				SELECT id
				FROM permissions
				WHERE permission_name = $1
			`, permissionName).Scan(&permissionID); err != nil {
				return err
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO role_permissions (role_id, permission_id, granted_at)
				VALUES ($1, $2, now())
				ON CONFLICT (role_id, permission_id) DO NOTHING
			`, roleID, permissionID); err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}
