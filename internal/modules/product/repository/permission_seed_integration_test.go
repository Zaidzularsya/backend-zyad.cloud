//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/platform/database/testutil"
)

func TestProductPermissionSeedIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	requiredPermissions := []string{
		"platform.product.plan.read",
		"platform.product.plan.manage",
		"platform.product.plan_price.read",
		"platform.product.plan_price.manage",
		"platform.product.feature.read",
		"platform.product.feature.manage",
		"platform.product.entitlement.read",
		"platform.product.entitlement.manage",
	}

	for _, permission := range requiredPermissions {
		var count int
		if err := db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM permissions
			WHERE permission_name = $1
				AND slug = $1
				AND module = 'product'
		`, permission).Scan(&count); err != nil {
			t.Fatalf("query permission %s: %v", permission, err)
		}
		if count != 1 {
			t.Fatalf("permission %s count = %d, want 1", permission, count)
		}
	}

	platformPermissions := []string{
		"platform.product.plan.read",
		"platform.product.plan.manage",
		"platform.product.plan_price.read",
		"platform.product.plan_price.manage",
	}
	for _, permission := range platformPermissions {
		var count int
		if err := db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM role_permissions rp
			JOIN roles r ON r.id = rp.role_id
			JOIN permissions p ON p.id = rp.permission_id
			WHERE (r.role_name = 'super_admin' OR r.slug = 'super_admin')
				AND p.permission_name = $1
				AND rp.scope = 'all'
		`, permission).Scan(&count); err != nil {
			t.Fatalf("query super_admin permission %s: %v", permission, err)
		}
		if count != 1 {
			t.Fatalf("super_admin permission %s count = %d, want 1", permission, count)
		}
	}
}
