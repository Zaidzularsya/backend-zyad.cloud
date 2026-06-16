//go:build integration

package repository_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/platform/database/testutil"
)

func TestOrganizationSelfPermissionSeedIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	permissions := []string{
		"organization.read",
		"organization.update",
		"organization.member.read",
		"organization.member.manage",
	}

	var permissionCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM permissions
		WHERE permission_name = ANY($1)
	`, permissions).Scan(&permissionCount); err != nil {
		t.Fatalf("count organization self permissions: %v", err)
	}
	if permissionCount != len(permissions) {
		t.Fatalf(
			"organization self permission count = %d, want %d",
			permissionCount,
			len(permissions),
		)
	}

	var grantCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM role_permissions role_permission
		JOIN roles role ON role.id = role_permission.role_id
		JOIN permissions permission ON permission.id = role_permission.permission_id
		WHERE (role.role_name = 'super_admin' OR role.slug = 'super_admin')
			AND permission.permission_name = ANY($1)
			AND role_permission.scope = 'organization'
	`, permissions).Scan(&grantCount); err != nil {
		t.Fatalf("count super admin organization self grants: %v", err)
	}
	if grantCount != len(permissions) {
		t.Fatalf(
			"super admin organization self grant count = %d, want %d",
			grantCount,
			len(permissions),
		)
	}
}
