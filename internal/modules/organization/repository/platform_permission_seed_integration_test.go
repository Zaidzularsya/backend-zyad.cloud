//go:build integration

package repository_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/platform/database/testutil"
)

func TestPlatformOrganizationPermissionSeedIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()

	var permissionCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM permissions
		WHERE permission_name = ANY($1)
	`, []string{
		"platform.organization.read",
		"platform.organization.manage",
		"platform.organization.suspend",
		"platform.organization.provision",
	}).Scan(&permissionCount); err != nil {
		t.Fatalf("count platform organization permissions: %v", err)
	}
	if permissionCount != 4 {
		t.Fatalf("platform organization permission count = %d, want 4", permissionCount)
	}

	var grantCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM role_permissions role_permission
		JOIN roles role ON role.id = role_permission.role_id
		JOIN permissions permission ON permission.id = role_permission.permission_id
		WHERE (role.role_name = 'super_admin' OR role.slug = 'super_admin')
			AND permission.permission_name = ANY($1)
			AND role_permission.scope = 'all'
	`, []string{
		"platform.organization.read",
		"platform.organization.manage",
		"platform.organization.suspend",
		"platform.organization.provision",
	}).Scan(&grantCount); err != nil {
		t.Fatalf("count super admin platform organization grants: %v", err)
	}
	if grantCount != 4 {
		t.Fatalf("super admin platform organization grant count = %d, want 4", grantCount)
	}
}
