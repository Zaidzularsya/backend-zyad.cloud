//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/platform/database/testutil"
)

func TestBillingPermissionSeedIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	requiredPermissions := []struct {
		name  string
		scope string
	}{
		{name: "platform.billing.plan.read", scope: "all"},
		{name: "platform.billing.plan.manage", scope: "all"},
		{name: "platform.billing.plan_price.read", scope: "all"},
		{name: "platform.billing.plan_price.manage", scope: "all"},
		{name: "platform.billing.feature.read", scope: "all"},
		{name: "platform.billing.feature.manage", scope: "all"},
		{name: "platform.billing.entitlement.read", scope: "all"},
		{name: "platform.billing.entitlement.manage", scope: "all"},
		{name: "platform.billing.subscription.read", scope: "all"},
		{name: "platform.billing.subscription.manage", scope: "all"},
		{name: "platform.billing.invoice.read", scope: "all"},
		{name: "platform.billing.invoice.manage", scope: "all"},
		{name: "platform.billing.payment.manage", scope: "all"},
		{name: "organization.billing.read", scope: "organization"},
		{name: "organization.billing.manage", scope: "organization"},
	}

	for _, permission := range requiredPermissions {
		var count int
		if err := db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM permissions
			WHERE permission_name = $1
				AND slug = $1
		`, permission.name).Scan(&count); err != nil {
			t.Fatalf("query permission %s: %v", permission.name, err)
		}
		if count != 1 {
			t.Fatalf("permission %s count = %d, want 1", permission.name, count)
		}
	}

	platformPermissions := []string{
		"platform.billing.plan.read",
		"platform.billing.plan.manage",
		"platform.billing.plan_price.read",
		"platform.billing.plan_price.manage",
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
