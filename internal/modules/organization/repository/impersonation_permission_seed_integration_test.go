//go:build integration

package repository_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/platform/database/testutil"
)

func TestPlatformImpersonationPermissionSeedIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()

	var permissionCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM permissions
		WHERE permission_name = 'platform.organization.impersonate'
	`).Scan(&permissionCount); err != nil {
		t.Fatalf("count platform impersonation permission: %v", err)
	}
	if permissionCount != 1 {
		t.Fatalf("platform impersonation permission count = %d, want 1", permissionCount)
	}

	var grantCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM role_permissions role_permission
		JOIN roles role ON role.id = role_permission.role_id
		JOIN permissions permission ON permission.id = role_permission.permission_id
		WHERE (role.role_name = 'super_admin' OR role.slug = 'super_admin')
			AND permission.permission_name = 'platform.organization.impersonate'
			AND role_permission.scope = 'all'
	`).Scan(&grantCount); err != nil {
		t.Fatalf("count super admin platform impersonation grant: %v", err)
	}
	if grantCount != 1 {
		t.Fatalf("super admin platform impersonation grant count = %d, want 1", grantCount)
	}
}
