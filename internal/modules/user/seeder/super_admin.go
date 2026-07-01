package seeder

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/core/auth"
	"zyad.cloud/internal/platform/database"
)

type Result struct {
	UserID string
	RoleID string
}

type PermissionSeed struct {
	Slug        string
	Description string
}

var basePermissions = []PermissionSeed{
	{Slug: "user.read", Description: "Read user data"},
	{Slug: "user.create", Description: "Create user"},
	{Slug: "user.update", Description: "Update user"},
	{Slug: "user.delete", Description: "Delete user"},
	{Slug: "user.restore", Description: "Restore deleted user"},
	{Slug: "user.update_status", Description: "Update user status"},
	{Slug: "user.session.read", Description: "Read user sessions"},
	{Slug: "user.session.revoke", Description: "Revoke user sessions"},
	{Slug: "role.read", Description: "Read role data"},
	{Slug: "role.create", Description: "Create role"},
	{Slug: "role.update", Description: "Update role"},
	{Slug: "role.delete", Description: "Delete role"},
	{Slug: "role.assign", Description: "Assign roles to users"},
	{Slug: "permission.read", Description: "Read permission data"},
	{Slug: "permission.manage", Description: "Manage permission data"},
	{Slug: "audit.read", Description: "Read audit logs"},
	{Slug: "organization.user.read", Description: "Read organization users"},
	{Slug: "organization.user.manage", Description: "Manage organization users"},
	{Slug: "platform.billing.plan.read", Description: "Read billing plan catalog"},
	{Slug: "platform.billing.plan.manage", Description: "Manage billing plans"},
	{Slug: "platform.billing.plan_price.read", Description: "Read billing plan prices"},
	{Slug: "platform.billing.plan_price.manage", Description: "Manage billing plan prices"},
	{Slug: "platform.billing.feature.read", Description: "Read billing feature catalog"},
	{Slug: "platform.billing.feature.manage", Description: "Manage billing feature catalog"},
	{Slug: "platform.billing.entitlement.read", Description: "Read billing plan entitlements"},
	{Slug: "platform.billing.entitlement.manage", Description: "Manage billing plan entitlements"},
	{Slug: "platform.billing.subscription.read", Description: "Read organization subscriptions"},
	{Slug: "platform.billing.subscription.manage", Description: "Manage organization subscriptions"},
	{Slug: "platform.billing.invoice.read", Description: "Read billing invoices"},
	{Slug: "platform.billing.invoice.manage", Description: "Manage billing invoices"},
	{Slug: "platform.billing.payment.manage", Description: "Manage billing payments"},
	{Slug: "organization.billing.read", Description: "Read current organization billing"},
	{Slug: "organization.billing.manage", Description: "Manage current organization billing requests"},
}

func SeedSuperAdmin(ctx context.Context, db *database.Pool, cfg config.SeedAdminConfig) (Result, error) {
	if db == nil {
		return Result{}, errors.New("database pool is required")
	}
	if err := cfg.Validate(); err != nil {
		return Result{}, err
	}

	passwordHash, err := auth.HashPassword(cfg.Password)
	if err != nil {
		return Result{}, fmt.Errorf("hash seed admin password: %w", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx)

	permissionIDs := make([]string, 0, len(basePermissions))
	for _, permission := range basePermissions {
		permissionID, err := upsertPermission(ctx, tx, permission)
		if err != nil {
			return Result{}, err
		}
		permissionIDs = append(permissionIDs, permissionID)
	}

	roleID, err := upsertRole(ctx, tx, cfg.Role)
	if err != nil {
		return Result{}, err
	}

	for _, permissionID := range permissionIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
			VALUES ($1, $2, 'all', now())
			ON CONFLICT (role_id, permission_id) DO UPDATE
			SET scope = EXCLUDED.scope
		`, roleID, permissionID); err != nil {
			return Result{}, fmt.Errorf("assign permission to seed role: %w", err)
		}
	}

	userID, err := upsertSuperAdminUser(ctx, tx, cfg, passwordHash)
	if err != nil {
		return Result{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_profiles (user_id, created_at, updated_at)
		VALUES ($1, now(), now())
		ON CONFLICT (user_id) DO NOTHING
	`, userID); err != nil {
		return Result{}, fmt.Errorf("create seed admin profile: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO auth_identities (user_id, provider, provider_user_id, provider_email, created_at, updated_at)
		VALUES ($1, 'local', $2, $3, now(), now())
		ON CONFLICT (provider, provider_user_id)
		DO UPDATE SET provider_email = EXCLUDED.provider_email, updated_at = now()
	`, userID, cfg.Username, cfg.Email); err != nil {
		return Result{}, fmt.Errorf("create seed admin local identity: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id, assigned_by, assigned_at)
		VALUES ($1, $2, $1, now())
		ON CONFLICT (user_id, role_id) WHERE organization_id IS NULL DO UPDATE
		SET assigned_by = EXCLUDED.assigned_by, assigned_at = EXCLUDED.assigned_at
	`, userID, roleID); err != nil {
		return Result{}, fmt.Errorf("assign seed role to seed admin: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}

	return Result{UserID: userID, RoleID: roleID}, nil
}

func upsertPermission(ctx context.Context, tx pgx.Tx, seed PermissionSeed) (string, error) {
	module, action := splitPermissionSlug(seed.Slug)

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO permissions (permission_name, module, action, name, slug, description, created_at, updated_at)
		VALUES ($1, $2, $3, $1, $1, $4, now(), now())
		ON CONFLICT (permission_name)
		DO UPDATE SET
			module = EXCLUDED.module,
			action = EXCLUDED.action,
			name = EXCLUDED.name,
			slug = EXCLUDED.slug,
			description = EXCLUDED.description,
			updated_at = now()
		RETURNING id
	`, seed.Slug, module, action, seed.Description).Scan(&id); err != nil {
		return "", fmt.Errorf("upsert permission %s: %w", seed.Slug, err)
	}

	return id, nil
}

func upsertRole(ctx context.Context, tx pgx.Tx, role string) (string, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		role = "super_admin"
	}

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO roles (role_name, slug, description, is_system, created_at, updated_at)
		VALUES ($1, $1, 'Full platform administrator', true, now(), now())
		ON CONFLICT (role_name)
		DO UPDATE SET
			slug = EXCLUDED.slug,
			description = EXCLUDED.description,
			is_system = true,
			updated_at = now()
		RETURNING id
	`, role).Scan(&id); err != nil {
		return "", fmt.Errorf("upsert seed role: %w", err)
	}

	return id, nil
}

func upsertSuperAdminUser(ctx context.Context, tx pgx.Tx, cfg config.SeedAdminConfig, passwordHash string) (string, error) {
	var existingID string
	err := tx.QueryRow(ctx, `
		SELECT id
		FROM users
		WHERE lower(email) = lower($1)
			AND deleted_at IS NULL
		LIMIT 1
	`, cfg.Email).Scan(&existingID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find seed admin user: %w", err)
	}

	if existingID != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE users
			SET
				name = $2,
				username = $3,
				password_hash = $4,
				phone = $5,
				status = 'active',
				email_verified_at = COALESCE(email_verified_at, now()),
				updated_at = now()
			WHERE id = $1
		`, existingID, cfg.Name, cfg.Username, passwordHash, cfg.WhatsApp); err != nil {
			return "", fmt.Errorf("update seed admin user: %w", err)
		}
		return existingID, nil
	}

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (
			name,
			email,
			username,
			password_hash,
			phone,
			status,
			email_verified_at,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, 'active', now(), now(), now())
		RETURNING id
	`, cfg.Name, cfg.Email, cfg.Username, passwordHash, cfg.WhatsApp).Scan(&id); err != nil {
		return "", fmt.Errorf("create seed admin user: %w", err)
	}

	return id, nil
}

func splitPermissionSlug(slug string) (string, string) {
	module, action, ok := strings.Cut(slug, ".")
	if !ok || module == "" || action == "" {
		return "permission", "manage"
	}
	return module, action
}
