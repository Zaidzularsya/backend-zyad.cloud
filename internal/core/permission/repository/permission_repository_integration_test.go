//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"zyad.cloud/internal/core/permission/domain"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestGetUserOrganizationPermissionsIsolatesScopesIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	suffix := strings.ReplaceAll(testutil.UniqueCode("permission-scope"), ".", "-")
	repository := New(db)

	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, status)
		VALUES ('Permission Scope User', $1, 'active')
		RETURNING id
	`, fmt.Sprintf("permission-scope-%s@example.test", suffix)).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}

	organizationIDs := make([]string, 2)
	for index := range organizationIDs {
		if err := db.QueryRow(ctx, `
			INSERT INTO organizations (
				type,
				slug,
				name,
				status,
				data_placement
			)
			VALUES ('customer', $1, $2, 'active', 'shared')
			RETURNING id
		`, fmt.Sprintf("permission-scope-%d-%s", index, suffix),
			fmt.Sprintf("Permission Scope Organization %d", index+1)).
			Scan(&organizationIDs[index]); err != nil {
			t.Fatalf("create organization %d: %v", index, err)
		}
	}

	permissionIDs := make(map[string]string)
	for _, slug := range []string{
		"platform.organization.read",
		"organization.member.read",
	} {
		module, action, _ := strings.Cut(slug, ".")
		var permissionID string
		if err := db.QueryRow(ctx, `
			INSERT INTO permissions (
				permission_name,
				name,
				slug,
				module,
				action
			)
			VALUES ($1, $2, $1, $3, $4)
			RETURNING id
		`, slug, slug+" "+suffix, module, action).Scan(&permissionID); err != nil {
			t.Fatalf("create permission %s: %v", slug, err)
		}
		permissionIDs[slug] = permissionID
	}

	roleIDs := make(map[string]string)
	for _, scope := range []string{"global", "organization-a", "organization-b"} {
		var roleID string
		if err := db.QueryRow(ctx, `
			INSERT INTO roles (role_name, slug, description)
			VALUES ($1, $2, 'Permission scope integration role')
			RETURNING id
		`, scope+" "+suffix, scope+"-"+suffix).Scan(&roleID); err != nil {
			t.Fatalf("create role %s: %v", scope, err)
		}
		roleIDs[scope] = roleID
	}

	if _, err := db.Exec(ctx, `
		INSERT INTO role_permissions (role_id, permission_id, scope)
		VALUES
			($1, $2, 'all'),
			($3, $4, 'organization'),
			($5, $4, 'organization')
	`, roleIDs["global"], permissionIDs["platform.organization.read"],
		roleIDs["organization-a"], permissionIDs["organization.member.read"],
		roleIDs["organization-b"]); err != nil {
		t.Fatalf("assign role permissions: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id, organization_id)
		VALUES
			($1, $2, NULL),
			($1, $3, $5),
			($1, $4, $6)
	`, userID, roleIDs["global"], roleIDs["organization-a"],
		roleIDs["organization-b"], organizationIDs[0], organizationIDs[1]); err != nil {
		t.Fatalf("assign user roles: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_permissions (
			user_id,
			permission_id,
			organization_id,
			effect
		)
		VALUES ($1, $2, $3, 'deny')
	`, userID, permissionIDs["organization.member.read"], organizationIDs[1]); err != nil {
		t.Fatalf("assign organization deny: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM user_permissions WHERE user_id = $1`, userID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM user_roles WHERE user_id = $1`, userID)
		for _, roleID := range roleIDs {
			_, _ = db.Exec(cleanupCtx, `DELETE FROM roles WHERE id = $1`, roleID)
		}
		for _, permissionID := range permissionIDs {
			_, _ = db.Exec(cleanupCtx, `DELETE FROM permissions WHERE id = $1`, permissionID)
		}
		for _, organizationID := range organizationIDs {
			_, _ = db.Exec(cleanupCtx, `DELETE FROM organizations WHERE id = $1`, organizationID)
		}
		_, _ = db.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID)
	})

	globalPermissions, err := repository.GetUserPermissions(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserPermissions() error = %v", err)
	}
	assertPermissionSlugs(t, globalPermissions, "platform.organization.read")

	organizationAPermissions, err := repository.GetUserOrganizationPermissions(
		ctx,
		userID,
		organizationIDs[0],
	)
	if err != nil {
		t.Fatalf("GetUserOrganizationPermissions(A) error = %v", err)
	}
	assertPermissionSlugs(t, organizationAPermissions, "organization.member.read")

	organizationBPermissions, err := repository.GetUserOrganizationPermissions(
		ctx,
		userID,
		organizationIDs[1],
	)
	if err != nil {
		t.Fatalf("GetUserOrganizationPermissions(B) error = %v", err)
	}
	assertPermissionSlugs(t, organizationBPermissions)
}

func assertPermissionSlugs(
	t *testing.T,
	permissionSet domain.UserPermissionSet,
	expected ...string,
) {
	t.Helper()
	actual := make(map[string]struct{}, len(permissionSet.Permissions))
	for _, permission := range permissionSet.Permissions {
		actual[permission.Slug] = struct{}{}
	}
	if len(actual) != len(expected) {
		t.Fatalf("permission slugs = %#v, want %#v", actual, expected)
	}
	for _, slug := range expected {
		if _, ok := actual[slug]; !ok {
			t.Fatalf("permission slugs = %#v, missing %q", actual, slug)
		}
	}
}
