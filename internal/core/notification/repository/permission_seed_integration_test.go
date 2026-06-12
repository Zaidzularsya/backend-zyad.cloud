//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"zyad.cloud/internal/platform/database/testutil"
)

func TestNotificationPermissionSeedIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	requiredPermissions := []string{
		"notification_template.read",
		"notification_template.create",
		"notification_template.update",
		"notification_template.delete",
		"notification_template.preview",
		"notification_template.activate",
		"notification_template.deactivate",
		"notification_template.archive",
		"notification_template.clone",
		"notification_variable.read",
		"notification_log.read",
		"notification_log.retry",
		"notification_log.cancel",
		"notification_preference.read",
		"notification_preference.update",
		"notification_preference.manage",
	}

	for _, permission := range requiredPermissions {
		var count int
		if err := db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM permissions
			WHERE permission_name = $1
				AND slug = $1
		`, permission).Scan(&count); err != nil {
			t.Fatalf("query permission %s: %v", permission, err)
		}
		if count != 1 {
			t.Fatalf("permission %s count = %d, want 1", permission, count)
		}

		if err := db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM role_permissions rp
			JOIN roles r ON r.id = rp.role_id
			JOIN permissions p ON p.id = rp.permission_id
			WHERE r.role_name = 'super_admin'
				AND p.permission_name = $1
				AND rp.scope = 'all'
		`, permission).Scan(&count); err != nil {
			t.Fatalf("query super admin permission %s: %v", permission, err)
		}
		if count != 1 {
			t.Fatalf("super_admin permission %s count = %d, want 1", permission, count)
		}
	}
}
