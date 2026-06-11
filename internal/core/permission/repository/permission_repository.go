package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"zyad.cloud/internal/core/permission/domain"
	"zyad.cloud/internal/platform/database"
)

type Repository struct {
	db *database.Pool
}

func New(db *database.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, permission_name, COALESCE(description, ''), COALESCE(module_id::text, ''), created_at, updated_at
		FROM permissions
		ORDER BY permission_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	permissions := make([]domain.Permission, 0)
	for rows.Next() {
		var permission domain.Permission
		if err := rows.Scan(
			&permission.ID,
			&permission.Name,
			&permission.Description,
			&permission.ModuleID,
			&permission.CreatedAt,
			&permission.UpdatedAt,
		); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}

	return permissions, rows.Err()
}

func (r *Repository) GetRolePermissions(ctx context.Context, roleName string) (domain.Role, error) {
	var role domain.Role
	if err := r.db.QueryRow(ctx, `
		SELECT id, role_name, COALESCE(description, ''), created_at, updated_at
		FROM roles
		WHERE role_name = $1
	`, roleName).Scan(
		&role.ID,
		&role.Name,
		&role.Description,
		&role.CreatedAt,
		&role.UpdatedAt,
	); err != nil {
		return domain.Role{}, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			p.id,
			p.permission_name,
			COALESCE(p.description, ''),
			COALESCE(p.module_id::text, ''),
			p.created_at,
			p.updated_at
		FROM role_permissions rp
		JOIN permissions p ON p.id = rp.permission_id
		JOIN roles r ON r.id = rp.role_id
		WHERE r.role_name = $1
		ORDER BY p.permission_name
	`, roleName)
	if err != nil {
		return domain.Role{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var permission domain.Permission
		if err := rows.Scan(
			&permission.ID,
			&permission.Name,
			&permission.Description,
			&permission.ModuleID,
			&permission.CreatedAt,
			&permission.UpdatedAt,
		); err != nil {
			return domain.Role{}, err
		}
		role.Permissions = append(role.Permissions, permission)
	}

	if err := rows.Err(); err != nil {
		return domain.Role{}, err
	}

	return role, nil
}

func (r *Repository) GetUserPermissions(ctx context.Context, userID string) (domain.UserPermissionSet, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT
			r.role_name,
			p.id,
			p.permission_name,
			COALESCE(p.description, ''),
			COALESCE(p.module_id::text, ''),
			p.created_at,
			p.updated_at
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		JOIN role_permissions rp ON rp.role_id = r.id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1
		ORDER BY r.role_name, p.permission_name
	`, userID)
	if err != nil {
		return domain.UserPermissionSet{}, err
	}
	defer rows.Close()

	result := domain.UserPermissionSet{UserID: userID}
	roleNames := map[string]struct{}{}
	permissionNames := map[string]struct{}{}

	for rows.Next() {
		var roleName string
		var permission domain.Permission
		if err := rows.Scan(
			&roleName,
			&permission.ID,
			&permission.Name,
			&permission.Description,
			&permission.ModuleID,
			&permission.CreatedAt,
			&permission.UpdatedAt,
		); err != nil {
			return domain.UserPermissionSet{}, err
		}

		if _, ok := roleNames[roleName]; !ok {
			roleNames[roleName] = struct{}{}
			result.RoleNames = append(result.RoleNames, roleName)
		}
		if _, ok := permissionNames[permission.Name]; !ok {
			permissionNames[permission.Name] = struct{}{}
			result.Permissions = append(result.Permissions, permission)
		}
	}

	return result, rows.Err()
}

func (r *Repository) GetPermissionByID(ctx context.Context, id string) (domain.Permission, error) {
	var permission domain.Permission
	err := r.db.QueryRow(ctx, `
		SELECT id, permission_name, COALESCE(description, ''), COALESCE(module_id::text, ''), created_at, updated_at
		FROM permissions
		WHERE id = $1
	`, id).Scan(
		&permission.ID,
		&permission.Name,
		&permission.Description,
		&permission.ModuleID,
		&permission.CreatedAt,
		&permission.UpdatedAt,
	)
	return permission, err
}

func (r *Repository) CreatePermission(ctx context.Context, p *domain.Permission) error {
	var moduleID *string
	if p.ModuleID != "" {
		moduleID = &p.ModuleID
	}

	return r.db.QueryRow(ctx, `
		INSERT INTO permissions (permission_name, description, module_id, created_at, updated_at)
		VALUES ($1, $2, $3, now(), now())
		RETURNING id, created_at, updated_at
	`, p.Name, p.Description, moduleID).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *Repository) UpdatePermission(ctx context.Context, id string, p *domain.Permission) error {
	var moduleID *string
	if p.ModuleID != "" {
		moduleID = &p.ModuleID
	}

	return r.db.QueryRow(ctx, `
		UPDATE permissions
		SET description = $1, module_id = $2, updated_at = now()
		WHERE id = $3
		RETURNING permission_name, created_at, updated_at
	`, p.Description, moduleID, id).Scan(&p.Name, &p.CreatedAt, &p.UpdatedAt)
}

func (r *Repository) DeletePermission(ctx context.Context, id string) error {
	res, err := r.db.Exec(ctx, `
		DELETE FROM permissions WHERE id = $1
	`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) GetRoleByID(ctx context.Context, id string) (domain.Role, error) {
	var role domain.Role
	err := r.db.QueryRow(ctx, `
		SELECT id, role_name, COALESCE(description, ''), created_at, updated_at
		FROM roles
		WHERE id = $1
	`, id).Scan(
		&role.ID,
		&role.Name,
		&role.Description,
		&role.CreatedAt,
		&role.UpdatedAt,
	)
	if err != nil {
		return domain.Role{}, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.permission_name, COALESCE(p.description, ''), COALESCE(p.module_id::text, ''), p.created_at, p.updated_at
		FROM role_permissions rp
		JOIN permissions p ON p.id = rp.permission_id
		WHERE rp.role_id = $1
		ORDER BY p.permission_name
	`, id)
	if err != nil {
		return domain.Role{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var p domain.Permission
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.ModuleID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return domain.Role{}, err
		}
		role.Permissions = append(role.Permissions, p)
	}

	return role, rows.Err()
}

func (r *Repository) ListRoles(ctx context.Context) ([]domain.Role, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, role_name, COALESCE(description, ''), created_at, updated_at
		FROM roles
		ORDER BY role_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := make([]domain.Role, 0)
	for rows.Next() {
		var role domain.Role
		if err := rows.Scan(
			&role.ID,
			&role.Name,
			&role.Description,
			&role.CreatedAt,
			&role.UpdatedAt,
		); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}

	return roles, rows.Err()
}

func (r *Repository) CreateRole(ctx context.Context, role *domain.Role) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO roles (role_name, description, created_at, updated_at)
		VALUES ($1, $2, now(), now())
		RETURNING id, created_at, updated_at
	`, role.Name, role.Description).Scan(&role.ID, &role.CreatedAt, &role.UpdatedAt)
}

func (r *Repository) UpdateRole(ctx context.Context, id string, role *domain.Role) error {
	return r.db.QueryRow(ctx, `
		UPDATE roles
		SET description = $1, updated_at = now()
		WHERE id = $2
		RETURNING role_name, created_at, updated_at
	`, role.Description, id).Scan(&role.Name, &role.CreatedAt, &role.UpdatedAt)
}

func (r *Repository) DeleteRole(ctx context.Context, id string) error {
	res, err := r.db.Exec(ctx, `
		DELETE FROM roles WHERE id = $1
	`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) AssignPermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, permID := range permissionIDs {
		_, err := tx.Exec(ctx, `
			INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
			VALUES ($1, $2, 'organization', now())
			ON CONFLICT (role_id, permission_id) DO NOTHING
		`, roleID, permID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) RevokePermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, permID := range permissionIDs {
		_, err := tx.Exec(ctx, `
			DELETE FROM role_permissions
			WHERE role_id = $1 AND permission_id = $2
		`, roleID, permID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) AssignUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, roleID := range roleIDs {
		_, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role_id, assigned_at)
			VALUES ($1, $2, now())
			ON CONFLICT (user_id, role_id) DO NOTHING
		`, userID, roleID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) RevokeUserRoles(ctx context.Context, userID string, roleIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, roleID := range roleIDs {
		_, err := tx.Exec(ctx, `
			DELETE FROM user_roles
			WHERE user_id = $1 AND role_id = $2
		`, userID, roleID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

