//go:build integration

package repository_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/platform/database/testutil"
)

func TestOrganizationDomainPermissionSeedIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()

	var grantCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM role_permissions role_permission
		JOIN roles role ON role.id = role_permission.role_id
		JOIN permissions permission ON permission.id = role_permission.permission_id
		WHERE (role.role_name = 'super_admin' OR role.slug = 'super_admin')
			AND permission.permission_name = 'organization.domain.manage'
			AND role_permission.scope = 'organization'
	`).Scan(&grantCount); err != nil {
		t.Fatalf("count organization domain grants: %v", err)
	}
	if grantCount != 1 {
		t.Fatalf("organization domain grant count = %d, want 1", grantCount)
	}
}
